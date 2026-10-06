package org.modlistdiscovery.client;

import static org.junit.jupiter.api.Assertions.*;

import com.sun.net.httpserver.HttpServer;
import java.net.*;
import java.nio.file.*;
import java.security.*;
import java.util.*;
import java.util.concurrent.atomic.*;
import org.junit.jupiter.api.*;
import org.junit.jupiter.api.io.TempDir;
import org.modlistdiscovery.client.core.*;
import org.modlistdiscovery.client.core.Models.*;

class SyncIntegrationTest {
  @TempDir Path dir;

  @Test
  void signedSyncUserConflictUpdateTamperAndRestart() throws Exception {
    byte[] v1 = CoreTest.jar("example", "1.0"), v2 = CoreTest.jar("example", "2.0");
    Manifest initial = CoreTest.manifest(v1), update = CoreTest.manifest(v2);
    update.release.id = 2;
    update.mods.getFirst().version = "2.0";
    KeyPair key = KeyPairGenerator.getInstance("Ed25519").generateKeyPair();
    AtomicReference<byte[]> manifest = new AtomicReference<>(CoreTest.sign(initial, key));
    AtomicBoolean corrupt = new AtomicBoolean();
    HttpServer server = HttpServer.create(new InetSocketAddress("127.0.0.1", 0), 0);
    server.createContext(
        "/test/mod-list.json",
        x -> {
          byte[] body = manifest.get();
          x.sendResponseHeaders(200, body.length);
          try (var out = x.getResponseBody()) {
            out.write(body);
          }
        });
    Map<String, byte[]> artifacts = Map.of(FilesEx.hash(v1), v1, FilesEx.hash(v2), v2);
    server.createContext(
        "/api/v1/servers/test/artifacts/",
        x -> {
          String hash = x.getRequestURI().getPath().split("/")[6];
          byte[] body = artifacts.get(hash).clone();
          if (corrupt.get()) body[0] ^= 1;
          x.sendResponseHeaders(200, body.length);
          try (var out = x.getResponseBody()) {
            out.write(body);
          }
        });
    server.start();
    Path mods = dir.resolve("mods");
    String url = "http://127.0.0.1:" + server.getAddress().getPort() + "/test";
    try {
      Files.createDirectories(mods);
      Path user = mods.resolve("user-example.jar");
      byte[] userBytes = CoreTest.jar("example", "0.9");
      Files.write(user, userBytes);
      try (Engine engine = new Engine(dir, mods, Set.of(), "26.2", "0.19.5")) {
        Profile profile = engine.add(url);
        Plan plan = engine.check(profile);
        assertTrue(plan.confirmationRequired());
        assertThrows(Exception.class, () -> engine.apply(plan, false, s -> {}));
        assertArrayEquals(userBytes, Files.readAllBytes(user));
        corrupt.set(true);
        assertThrows(Exception.class, () -> engine.apply(plan, true, s -> {}));
        assertArrayEquals(userBytes, Files.readAllBytes(user));
        assertFalse(engine.recoveryRequired());
        corrupt.set(false);
        engine.apply(plan, true, s -> {});
        assertFalse(Files.exists(user));
        assertEquals(1, engine.quarantined().size());
        assertTrue(engine.restartRequired);
        assertEquals(Status.RESTART_REQUIRED, engine.status(profile, engine.check(profile)));
        assertEquals(FilesEx.hash(v1), engine.state.managedFiles.getFirst().sha256);
      }
      try (Engine engine = new Engine(dir, mods, Set.of(), "26.2", "0.19.5")) {
        Profile p = engine.profiles.profiles.getFirst();
        assertEquals(Status.READY, engine.status(p, engine.check(p)));
        manifest.set(CoreTest.sign(update, key));
        Plan plan = engine.check(p);
        assertFalse(plan.confirmationRequired());
        assertTrue(plan.items().stream().anyMatch(i -> i.action() == Action.UPDATE));
        engine.apply(plan, false, s -> {});
        assertEquals(2, engine.quarantined().size());
        assertEquals(FilesEx.hash(v2), engine.state.managedFiles.getFirst().sha256);
        manifest.set(CoreTest.sign(initial, key));
        engine.apply(engine.check(p), false, s -> {});
        assertEquals(FilesEx.hash(v1), engine.state.managedFiles.getFirst().sha256);
        manifest.set(
            CoreTest.sign(initial, KeyPairGenerator.getInstance("Ed25519").generateKeyPair()));
        assertThrows(java.io.IOException.class, () -> engine.check(p));
      }
    } finally {
      server.stop(0);
    }
  }

  @Test
  void authorizationNeverCrossesOrigin() throws Exception {
    byte[] bytes = CoreTest.jar("example", "1.0");
    AtomicReference<String> received = new AtomicReference<>();
    AtomicReference<String> control = new AtomicReference<>();
    HttpServer object = HttpServer.create(new InetSocketAddress("127.0.0.1", 0), 0),
        origin = HttpServer.create(new InetSocketAddress("127.0.0.1", 0), 0);
    object.createContext(
        "/file",
        x -> {
          received.set(x.getRequestHeaders().getFirst("Authorization"));
          x.sendResponseHeaders(200, bytes.length);
          try (var out = x.getResponseBody()) {
            out.write(bytes);
          }
        });
    object.start();
    origin.createContext(
        "/download",
        x -> {
          control.set(x.getRequestHeaders().getFirst("Authorization"));
          x.getResponseHeaders()
              .set("Location", "http://127.0.0.1:" + object.getAddress().getPort() + "/file");
          x.sendResponseHeaders(302, -1);
          x.close();
        });
    origin.start();
    try (Network network = new Network()) {
      Artifact artifact = CoreTest.manifest(bytes).artifacts.values().iterator().next();
      network.download(
          URI.create("http://127.0.0.1:" + origin.getAddress().getPort() + "/download"),
          "secret-device-token",
          artifact,
          FilesEx.hash(bytes),
          dir);
      assertEquals("Bearer secret-device-token", control.get());
      assertNull(received.get());
    } finally {
      origin.stop(0);
      object.stop(0);
    }
  }
}
