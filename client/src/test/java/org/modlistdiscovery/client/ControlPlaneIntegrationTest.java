package org.modlistdiscovery.client;

import static org.junit.jupiter.api.Assertions.*;

import com.google.gson.*;
import java.io.*;
import java.net.*;
import java.net.http.*;
import java.nio.charset.StandardCharsets;
import java.nio.file.*;
import java.time.Duration;
import java.util.*;
import org.junit.jupiter.api.*;
import org.junit.jupiter.api.condition.EnabledIfEnvironmentVariable;
import org.junit.jupiter.api.io.TempDir;
import org.modlistdiscovery.client.core.*;
import org.modlistdiscovery.client.core.Models.*;

@EnabledIfEnvironmentVariable(named = "MLD_SERVER_BINARY", matches = ".+")
class ControlPlaneIntegrationTest {
  @TempDir Path dir;
  private HttpClient http;
  private String origin, csrf;

  private JsonObject api(String method, String path, Object body, int status) throws Exception {
    var request = HttpRequest.newBuilder(URI.create(origin + path)).timeout(Duration.ofSeconds(10));
    if (csrf != null) request.header("X-CSRF-Token", csrf);
    request
        .header("Content-Type", "application/json")
        .method(
            method,
            body == null
                ? HttpRequest.BodyPublishers.noBody()
                : HttpRequest.BodyPublishers.ofString(FilesEx.JSON.toJson(body)));
    var response = http.send(request.build(), HttpResponse.BodyHandlers.ofString());
    assertEquals(status, response.statusCode(), response.body());
    return JsonParser.parseString(response.body()).getAsJsonObject();
  }

  @Test
  void realGoPrivateServerToJavaClient() throws Exception {
    String binary = Path.of(System.getenv("MLD_SERVER_BINARY")).toAbsolutePath().toString();
    int port;
    try (ServerSocket socket = new ServerSocket(0)) {
      port = socket.getLocalPort();
    }
    origin = "http://127.0.0.1:" + port;
    Path config = dir.resolve("config.yaml");
    Files.writeString(
        config,
        "server:\n  data_dir: "
            + dir.resolve("server")
            + "\n  listen: 127.0.0.1:"
            + port
            + "\n  public_base_url: "
            + origin
            + "\n");
    ProcessBuilder init = new ProcessBuilder(binary, "init", "--config", config.toString());
    init.environment().put("MLD_ADMIN_PASSWORD", "integration-test-password");
    init.redirectErrorStream(true).redirectOutput(dir.resolve("init.log").toFile());
    assertEquals(0, init.start().waitFor());
    Process process =
        new ProcessBuilder(binary, "serve", "--config", config.toString())
            .redirectErrorStream(true)
            .redirectOutput(dir.resolve("serve.log").toFile())
            .start();
    http =
        HttpClient.newBuilder()
            .cookieHandler(new CookieManager(null, CookiePolicy.ACCEPT_ALL))
            .connectTimeout(Duration.ofSeconds(1))
            .build();
    try {
      boolean healthy = false;
      for (int i = 0; i < 100; i++) {
        try {
          if (http.send(
                      HttpRequest.newBuilder(URI.create(origin + "/healthz")).build(),
                      HttpResponse.BodyHandlers.discarding())
                  .statusCode()
              == 200) {
            healthy = true;
            break;
          }
        } catch (IOException e) {
          if (!process.isAlive())
            throw new IOException(Files.readString(dir.resolve("serve.log")), e);
        }
        Thread.sleep(50);
      }
      assertTrue(healthy);
      csrf =
          api(
                  "POST",
                  "/api/v1/admin/login",
                  Map.of("username", "admin", "password", "integration-test-password"),
                  200)
              .get("csrfToken")
              .getAsString();
      api(
          "POST",
          "/api/v1/admin/servers",
          Map.of("id", "test", "displayName", "Interop", "visibility", "private"),
          201);
      byte[] jar = CoreTest.jar("example", "1.0");
      ByteArrayOutputStream multipart = new ByteArrayOutputStream();
      multipart.write(
          "--test-boundary\r\nContent-Disposition: form-data; name=\"file\"; filename=\"example.jar\"\r\nContent-Type: application/java-archive\r\n\r\n"
              .getBytes(StandardCharsets.UTF_8));
      multipart.write(jar);
      multipart.write("\r\n--test-boundary--\r\n".getBytes(StandardCharsets.UTF_8));
      var upload =
          http.send(
              HttpRequest.newBuilder(URI.create(origin + "/api/v1/admin/artifacts/upload"))
                  .header("X-CSRF-Token", csrf)
                  .header("Content-Type", "multipart/form-data; boundary=test-boundary")
                  .POST(HttpRequest.BodyPublishers.ofByteArray(multipart.toByteArray()))
                  .build(),
              HttpResponse.BodyHandlers.ofString());
      assertEquals(201, upload.statusCode(), upload.body());
      String hash =
          JsonParser.parseString(upload.body()).getAsJsonObject().get("sha256").getAsString();
      long release =
          api("POST", "/api/v1/admin/servers/test/releases", Map.of("name", "interop"), 201)
              .get("id")
              .getAsLong();
      api(
          "POST",
          "/api/v1/admin/releases/" + release + "/mods",
          Map.of("modId", "example", "requirement", "required", "artifact", hash),
          200);
      api("POST", "/api/v1/admin/releases/" + release + "/publish", Map.of(), 200);
      api("PUT", "/api/v1/admin/servers/test/channels/stable", Map.of("releaseId", release), 200);
      String code =
          api("POST", "/api/v1/admin/servers/test/enrollment-codes", Map.of("maxUses", 1), 201)
              .get("code")
              .getAsString();
      Path game = dir.resolve("game");
      try (Engine engine = new Engine(game, game.resolve("mods"), Set.of(), "26.2", "0.19.5")) {
        Profile p = engine.add(origin + "/test");
        assertThrows(IOException.class, () -> engine.check(p));
        engine.enroll(p, code, "Interop Java device");
        assertNotNull(p.trustedSigningKey);
        assertTrue(p.trustedSigningKey.keyId.startsWith("ed25519:"));
        Plan plan = engine.check(p);
        assertEquals("Interop", plan.manifest().server.displayName);
        engine.apply(plan, false, s -> {});
        assertEquals(hash, engine.state.managedFiles.getFirst().sha256);
        assertTrue(engine.restartRequired);
        api("POST", "/api/v1/admin/clients/" + p.deviceId + "/revoke", Map.of(), 200);
        assertThrows(IOException.class, () -> engine.check(p));
        assertThrows(IOException.class, () -> engine.apply(plan, false, s -> {}));
      }
    } finally {
      http.close();
      process.destroy();
      if (!process.waitFor(10, java.util.concurrent.TimeUnit.SECONDS)) process.destroyForcibly();
    }
  }
}
