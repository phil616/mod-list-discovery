package org.modlistdiscovery.client;

import static org.junit.jupiter.api.Assertions.*;

import com.google.gson.*;
import com.sun.net.httpserver.HttpServer;
import java.io.*;
import java.net.*;
import java.nio.charset.StandardCharsets;
import java.nio.file.*;
import java.security.*;
import java.time.Duration;
import java.util.*;
import java.util.zip.*;
import org.junit.jupiter.api.*;
import org.junit.jupiter.api.io.TempDir;
import org.modlistdiscovery.client.core.*;
import org.modlistdiscovery.client.core.Models.*;

class CoreTest {
  @TempDir Path dir;

  static byte[] jar(String id, String version) throws IOException {
    var out = new ByteArrayOutputStream();
    try (ZipOutputStream z = new ZipOutputStream(out)) {
      z.putNextEntry(new ZipEntry("fabric.mod.json"));
      z.write(
          ("{\"id\":\"" + id + "\",\"version\":\"" + version + "\"}")
              .getBytes(StandardCharsets.UTF_8));
      z.closeEntry();
    }
    return out.toByteArray();
  }

  static Profile profile() {
    Profile p = new Profile();
    p.serverId = "test";
    p.baseUrl = "https://example.com/test";
    p.displayName = "Test";
    return p;
  }

  static Manifest manifest(byte[] jar) {
    Manifest m = new Manifest();
    m.schemaVersion = 1;
    m.server = new Server();
    m.server.id = "test";
    m.server.displayName = "Test";
    m.channel = "stable";
    m.release = new Release();
    m.release.id = 1;
    m.release.name = "first";
    m.release.publishedAt = "2026-10-05T00:00:00Z";
    m.runtime = new Models.Runtime();
    m.runtime.minecraft = "26.2";
    m.runtime.fabricLoader = ">=0.19.0";
    m.runtime.java = ">=25";
    m.runtime.minDiscoveryVersion = Models.VERSION;
    Mod mod = new Mod();
    mod.modId = "example";
    mod.version = "1.0";
    mod.requirement = Requirement.required;
    mod.artifact = FilesEx.hash(jar);
    m.mods = new ArrayList<>(List.of(mod));
    Artifact a = new Artifact();
    a.size = jar.length;
    a.filename = "../../evil.jar";
    a.download = new Download();
    a.download.type = "api";
    a.download.path = "/api/v1/servers/test/artifacts/" + mod.artifact + "/download";
    m.artifacts = new HashMap<>();
    m.artifacts.put(mod.artifact, a);
    return m;
  }

  static byte[] sign(Manifest m, KeyPair key) throws Exception {
    byte[] payload = FilesEx.JSON.toJson(m).getBytes(StandardCharsets.UTF_8),
        raw = Arrays.copyOfRange(key.getPublic().getEncoded(), 12, 44);
    Signature sig = Signature.getInstance("Ed25519");
    sig.initSign(key.getPrivate());
    sig.update(payload);
    Base64.Encoder enc = Base64.getUrlEncoder().withoutPadding();
    return FilesEx.JSON
        .toJson(
            Map.of(
                "schemaVersion",
                1,
                "payloadEncoding",
                "base64url",
                "payload",
                enc.encodeToString(payload),
                "signature",
                Map.of(
                    "algorithm",
                    "Ed25519",
                    "keyId",
                    "ed25519:" + FilesEx.hash(raw).substring(0, 16),
                    "publicKey",
                    enc.encodeToString(raw),
                    "value",
                    enc.encodeToString(sig.sign()))))
        .getBytes(StandardCharsets.UTF_8);
  }

  @Test
  void urls() {
    assertEquals(
        "https://example.com/test",
        DiscoveryUrl.parse("https://EXAMPLE.com/test/").base().toString());
    for (String s :
        List.of("http://localhost/test", "http://127.0.0.1:123/test", "http://[::1]/test"))
      assertDoesNotThrow(() -> DiscoveryUrl.parse(s));
    for (String s :
        List.of(
            "http://example.com/test",
            "https://x/test?q=1",
            "https://x/test#fragment",
            "https://u:p@x/test",
            "https://x/a/b",
            "https://x/admin",
            "https://x/%74est",
            "https://x/test//"))
      assertThrows(IllegalArgumentException.class, () -> DiscoveryUrl.parse(s));
  }

  @Test
  void signatures() throws Exception {
    Manifest m = manifest(jar("example", "1.0"));
    Profile p = profile();
    KeyPair key = KeyPairGenerator.getInstance("Ed25519").generateKeyPair();
    byte[] encoded = sign(m, key);
    var verified = ManifestVerifier.verify(encoded, p);
    p.trustedSigningKey = verified.key();
    assertEquals(1, verified.manifest().release.id);
    assertDoesNotThrow(() -> ManifestVerifier.verify(encoded, p));
    var json =
        JsonParser.parseString(new String(encoded, StandardCharsets.UTF_8)).getAsJsonObject();
    json.addProperty(
        "payload",
        Base64.getUrlEncoder()
            .withoutPadding()
            .encodeToString("{}".getBytes(StandardCharsets.UTF_8)));
    assertThrows(
        IOException.class,
        () -> ManifestVerifier.verify(json.toString().getBytes(StandardCharsets.UTF_8), p));
    assertThrows(
        IOException.class,
        () ->
            ManifestVerifier.verify(
                sign(m, KeyPairGenerator.getInstance("Ed25519").generateKeyPair()), p));
    json.addProperty("payload", "bad===");
    assertThrows(
        IOException.class,
        () -> ManifestVerifier.verify(json.toString().getBytes(StandardCharsets.UTF_8), p));
    m.mods.getFirst().modId = Models.SELF;
    assertThrows(IOException.class, () -> ManifestVerifier.verify(sign(m, key), p));
  }

  @Test
  void scanAndPlan() throws Exception {
    Path mods = dir.resolve("mods");
    Files.createDirectories(mods);
    byte[] target = jar("example", "1.0");
    Manifest m = manifest(target);
    Profile p = profile();
    State state = new State();
    assertEquals(Action.INSTALL, Planner.plan(m, p, List.of()).items().getFirst().action());
    Path file = mods.resolve("user.jar");
    Files.write(file, jar("example", "0.9"));
    List<Local> scan = org.modlistdiscovery.client.core.Scanner.scan(mods, state, Set.of());
    assertFalse(scan.getFirst().isManaged());
    Plan plan = Planner.plan(m, p, scan);
    assertTrue(plan.confirmationRequired());
    assertTrue(plan.items().stream().anyMatch(i -> i.action() == Action.UPDATE));
    Managed managed = new Managed();
    managed.filename = "user.jar";
    managed.sha256 = FilesEx.hash(file);
    managed.modId = "example";
    managed.profileId = "other";
    state.managedFiles.add(managed);
    scan = org.modlistdiscovery.client.core.Scanner.scan(mods, state, Set.of());
    assertTrue(scan.getFirst().isManaged());
    assertFalse(Planner.plan(m, p, scan).confirmationRequired());
    Files.write(file, target);
    assertFalse(
        org.modlistdiscovery.client.core.Scanner.scan(mods, state, Set.of())
            .getFirst()
            .isManaged());
    assertEquals(
        Action.KEEP,
        Planner.plan(m, p, org.modlistdiscovery.client.core.Scanner.scan(mods, state, Set.of()))
            .items()
            .getFirst()
            .action());
    m.mods.getFirst().requirement = Requirement.optional;
    p.optionalSelections.put("example", false);
    assertTrue(Planner.plan(m, p, List.of()).items().isEmpty());
    p.optionalSelections.put("example", true);
    assertEquals(
        Action.OPTIONAL_INSTALL, Planner.plan(m, p, List.of()).items().getFirst().action());
    m.mods.getFirst().requirement = Requirement.forbidden;
    m.mods.getFirst().artifact = null;
    m.mods.getFirst().versionConstraint = ">=1.0";
    assertTrue(
        Planner.plan(m, p, org.modlistdiscovery.client.core.Scanner.scan(mods, state, Set.of()))
            .confirmationRequired());
    Files.write(mods.resolve("bad.jar"), new byte[] {1});
    assertTrue(
        Planner.plan(m, p, org.modlistdiscovery.client.core.Scanner.scan(mods, state, Set.of()))
            .blocked());
  }

  @Test
  void runtimeConstraints() throws Exception {
    Manifest m = manifest(jar("example", "1.0"));
    assertDoesNotThrow(() -> Planner.runtime(m, "26.2", "0.19.5", "25"));
    assertThrows(IOException.class, () -> Planner.runtime(m, "26.2", "0.9.0", "25"));
    assertTrue(Planner.matches(">=0.19.0", "0.100.0"));
  }

  @Test
  void stateAndRecovery() throws Exception {
    Path game = dir.resolve("game"), mods = game.resolve("mods");
    try (Engine e = new Engine(game, mods, Set.of(), "26.2", "0.19.5")) {
      Profile p = e.add("http://localhost/test");
      assertEquals(1, e.profiles.profiles.size());
      try (Engine second = new Engine(game, mods, Set.of(), "26.2", "0.19.5")) {
        assertTrue(second.readOnly);
        assertThrows(IOException.class, () -> second.add("http://localhost/another"));
      }
      byte[] bytes = jar("example", "1.0");
      Path original = mods.resolve("original.jar"),
          dest = e.root.resolve("quarantine/test/original.jar");
      Files.write(original, bytes);
      Journal journal = new Journal();
      journal.previousState = new State();
      journal.moves.add(new Move(original, dest, FilesEx.hash(bytes), false));
      FilesEx.atomic(e.root.resolve("recovery/current.json"), journal);
      FilesEx.move(original, dest, FilesEx.hash(bytes));
      assertTrue(e.recoveryRequired());
      e.rollback();
      assertArrayEquals(bytes, Files.readAllBytes(original));
      assertFalse(e.recoveryRequired());
      assertTrue(e.restartRequired);
      assertEquals(p.id, e.profiles.profiles.getFirst().id);
    }
    Path state = game.resolve("mod-list-discovery/state.json");
    Files.writeString(state, "{");
    assertThrows(IOException.class, () -> new Engine(game, mods, Set.of(), "26.2", "0.19.5"));
    assertEquals("{", Files.readString(state));
  }

  @Test
  void downloadsBoundedAndVerified() throws Exception {
    byte[] bytes = jar("example", "1.0");
    HttpServer server = HttpServer.create(new InetSocketAddress("127.0.0.1", 0), 0);
    server.createContext(
        "/ok",
        x -> {
          x.sendResponseHeaders(200, bytes.length);
          try (var body = x.getResponseBody()) {
            body.write(bytes);
          }
        });
    server.createContext(
        "/redirect",
        x -> {
          x.getResponseHeaders().set("Location", "/ok");
          x.sendResponseHeaders(302, -1);
          x.close();
        });
    server.createContext(
        "/slow",
        x -> {
          try {
            Thread.sleep(500);
            x.sendResponseHeaders(200, 0);
            x.close();
          } catch (InterruptedException ex) {
            Thread.currentThread().interrupt();
          }
        });
    server.start();
    try (Network n = new Network(Duration.ofMillis(150))) {
      String base = "http://127.0.0.1:" + server.getAddress().getPort();
      Artifact a = manifest(bytes).artifacts.values().iterator().next();
      String hash = FilesEx.hash(bytes);
      Path result =
          n.download(URI.create(base + "/redirect"), "test-token", a, hash, dir.resolve("one"));
      assertArrayEquals(bytes, Files.readAllBytes(result));
      a.size++;
      assertThrows(
          IOException.class,
          () -> n.download(URI.create(base + "/ok"), null, a, hash, dir.resolve("two")));
      a.size -= 2;
      assertThrows(
          IOException.class,
          () -> n.download(URI.create(base + "/ok"), null, a, hash, dir.resolve("three")));
      a.size = bytes.length;
      assertThrows(
          IOException.class,
          () -> n.download(URI.create(base + "/ok"), null, a, "a".repeat(64), dir.resolve("four")));
      assertThrows(
          IOException.class,
          () -> n.download(URI.create(base + "/slow"), null, a, hash, dir.resolve("five")));
    } finally {
      server.stop(0);
    }
  }

  @Test
  void safeFileNamesAndNoOverwrite() throws Exception {
    assertThrows(IOException.class, () -> FilesEx.child(dir, "../evil"));
    Path a = dir.resolve("a"), b = dir.resolve("b");
    Files.writeString(a, "source");
    Files.writeString(b, "user data");
    assertThrows(IOException.class, () -> FilesEx.move(a, b, FilesEx.hash(a)));
    assertEquals("user data", Files.readString(b));
    Path state = dir.resolve("state.json");
    FilesEx.atomic(state, new State());
    assertEquals(1, FilesEx.read(state, State.class, null).schemaVersion);
  }

  @Test
  void unrelatedDuplicateIdsCannotBeReady() throws Exception {
    Path mods = dir.resolve("mods");
    Files.createDirectories(mods);
    Files.write(mods.resolve("one.jar"), jar("user-extra", "1.0"));
    Files.write(mods.resolve("two.jar"), jar("user-extra", "2.0"));
    Manifest m = manifest(jar("example", "1.0"));
    m.mods.clear();
    m.artifacts.clear();
    var plan =
        Planner.plan(
            m,
            profile(),
            org.modlistdiscovery.client.core.Scanner.scan(mods, new State(), Set.of()));
    assertTrue(plan.blocked());
    assertFalse(plan.confirmationRequired());
    assertEquals("version-_", FilesEx.safe("version-\uD83D\uDE00"));
  }
}
