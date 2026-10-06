package org.modlistdiscovery.client.core;

import com.google.gson.JsonParser;
import java.io.*;
import java.net.URI;
import java.nio.channels.*;
import java.nio.file.*;
import java.time.Instant;
import java.util.*;
import java.util.concurrent.*;
import java.util.concurrent.locks.ReentrantLock;
import java.util.function.Consumer;
import org.modlistdiscovery.client.core.Models.*;

public final class Engine implements AutoCloseable {
  public final Path root, mods;
  public Profiles profiles;
  public State state;
  public volatile boolean restartRequired;
  public final boolean readOnly;
  private final FileChannel lockChannel, modsLockChannel;
  private final FileLock lock, modsLock;
  private final ReentrantLock operation = new ReentrantLock();
  private final Network network = new Network();
  private final Set<Path> selfOrigins;
  private final Map<String, Plan> issuedPlans = new HashMap<>();
  private final String startupSnapshot;
  private final String minecraftVersion, loaderVersion;

  public Engine(
      Path game, Path mods, Set<Path> origins, String minecraftVersion, String loaderVersion)
      throws IOException {
    this.root = game.resolve("mod-list-discovery").toAbsolutePath().normalize();
    this.mods = mods.toAbsolutePath().normalize();
    this.selfOrigins = Set.copyOf(origins);
    this.minecraftVersion = minecraftVersion;
    this.loaderVersion = loaderVersion;
    for (Path p :
        List.of(
            root,
            this.mods,
            root.resolve("cache"),
            root.resolve("staging"),
            root.resolve("quarantine"),
            root.resolve("logs"),
            root.resolve("recovery"))) {
      Files.createDirectories(p);
      if (Files.isSymbolicLink(p)) throw new IOException("UNSAFE_PATH");
    }
    lockChannel =
        FileChannel.open(
            root.resolve("client.lock"), StandardOpenOption.CREATE, StandardOpenOption.WRITE);
    lock = tryLock(lockChannel);
    modsLockChannel =
        FileChannel.open(
            this.mods.resolve(".mod-list-discovery.lock"),
            StandardOpenOption.CREATE,
            StandardOpenOption.WRITE);
    modsLock = tryLock(modsLockChannel);
    readOnly = lock == null || modsLock == null;
    try {
      profiles = FilesEx.read(root.resolve("profiles.json"), Profiles.class, new Profiles());
      state = FilesEx.read(root.resolve("state.json"), State.class, new State());
      if (profiles.schemaVersion != 1
          || profiles.profiles == null
          || state.schemaVersion != 1
          || state.managedFiles == null) throw new IOException("CORRUPTED_STATE");
      Set<String> profileIds = new HashSet<>();
      for (Profile p : profiles.profiles) {
        if (p == null
            || p.id == null
            || !p.id.matches("[a-zA-Z0-9-]{1,64}")
            || !profileIds.add(p.id)
            || p.optionalSelections == null
            || p.channel == null
            || !p.channel.matches("[a-z0-9][a-z0-9-]{0,63}")
            || !DiscoveryUrl.parse(p.baseUrl).serverId().equals(p.serverId))
          throw new IOException("CORRUPTED_STATE");
      }
      Set<String> managedNames = new HashSet<>();
      for (Managed m : state.managedFiles) {
        if (m == null
            || m.sha256 == null
            || !m.sha256.matches("[0-9a-f]{64}")
            || m.modId == null
            || Models.SELF.equals(m.modId)
            || !managedNames.add(m.filename)) throw new IOException("CORRUPTED_STATE");
        FilesEx.child(this.mods, m.filename);
      }
      startupSnapshot = Scanner.snapshot(Scanner.scan(this.mods, state, selfOrigins));
    } catch (IOException | RuntimeException e) {
      close();
      throw e;
    }
  }

  private static FileLock tryLock(FileChannel c) throws IOException {
    try {
      return c.tryLock();
    } catch (OverlappingFileLockException e) {
      return null;
    }
  }

  private void begin() throws IOException {
    if (!operation.tryLock()) throw new IOException("OPERATION_IN_PROGRESS");
  }

  private void writable() throws IOException {
    if (readOnly) throw new IOException("READ_ONLY");
  }

  public boolean recoveryRequired() {
    return Files.exists(root.resolve("recovery/current.json"));
  }

  public void saveProfiles() throws IOException {
    writable();
    FilesEx.atomic(root.resolve("profiles.json"), profiles);
  }

  public Profile add(String url) throws IOException {
    begin();
    try {
      writable();
      DiscoveryUrl parsed = DiscoveryUrl.parse(url);
      for (Profile p : profiles.profiles) if (p.baseUrl.equals(parsed.base().toString())) return p;
      Profile p = new Profile();
      p.baseUrl = parsed.base().toString();
      p.serverId = parsed.serverId();
      p.displayName = p.serverId;
      profiles.profiles.add(p);
      saveProfiles();
      return p;
    } finally {
      operation.unlock();
    }
  }

  public void remove(Profile p) throws IOException {
    begin();
    try {
      writable();
      if (recoveryRequired()) throw new IOException("RECOVERY_REQUIRED");
      if (p.id.equals(profiles.activeProfileId))
        throw new IOException("ACTIVE_PROFILE_CANNOT_BE_REMOVED");
      profiles.profiles.remove(p);
      saveProfiles();
      issuedPlans.remove(p.id);
    } finally {
      operation.unlock();
    }
  }

  public void enroll(Profile p, String code, String device)
      throws IOException, InterruptedException {
    begin();
    try {
      writable();
      DiscoveryUrl u = DiscoveryUrl.parse(p.baseUrl);
      byte[] b =
          network.json(
              u.endpoint("/enroll"),
              null,
              FilesEx.JSON.toJson(Map.of("code", code, "deviceName", device)),
              root.resolve("staging"));
      var response =
          JsonParser.parseString(new String(b, java.nio.charset.StandardCharsets.UTF_8))
              .getAsJsonObject();
      if (!p.serverId.equals(response.get("serverId").getAsString()))
        throw new IOException("INVALID_ENROLLMENT");
      SigningKey key = FilesEx.JSON.fromJson(response.get("signingKey"), SigningKey.class);
      try {
        byte[] raw = Base64.getUrlDecoder().decode(key.publicKey);
        if (!"Ed25519".equals(response.getAsJsonObject("signingKey").get("algorithm").getAsString())
            || raw.length != 32
            || !key.keyId.equals("ed25519:" + FilesEx.hash(raw).substring(0, 16)))
          throw new IOException("INVALID_ENROLLMENT");
      } catch (IllegalArgumentException | NullPointerException e) {
        throw new IOException("INVALID_ENROLLMENT", e);
      }
      if (p.trustedSigningKey != null && !p.trustedSigningKey.publicKey.equals(key.publicKey))
        throw new IOException("SIGNING_KEY_CHANGED");
      p.trustedSigningKey = key;
      p.accessToken = response.get("accessToken").getAsString();
      p.deviceId = response.get("deviceId").getAsString();
      saveProfiles();
    } finally {
      operation.unlock();
    }
  }

  public Plan check(Profile p) throws IOException, InterruptedException {
    begin();
    try {
      issuedPlans.remove(p.id);
      if (recoveryRequired()) throw new IOException("RECOVERY_REQUIRED");
      DiscoveryUrl u = DiscoveryUrl.parse(p.baseUrl);
      URI uri = u.manifest();
      if (!p.channel.equals("stable")) uri = URI.create(uri + "?channel=" + p.channel);
      ManifestVerifier.Verified verified =
          ManifestVerifier.verify(
              network.json(uri, p.accessToken, null, root.resolve("staging")), p);
      Planner.runtime(
          verified.manifest(),
          minecraftVersion,
          loaderVersion,
          Integer.toString(java.lang.Runtime.version().feature()));
      p.trustedSigningKey = verified.key();
      p.displayName = verified.manifest().server.displayName;
      p.lastSeenReleaseId = verified.manifest().release.id;
      if (!readOnly) saveProfiles();
      List<Local> scan = Scanner.scan(mods, state, selfOrigins);
      if (!Scanner.snapshot(scan).equals(startupSnapshot)) restartRequired = true;
      Plan plan = Planner.plan(verified.manifest(), p, scan);
      issuedPlans.put(p.id, plan);
      return plan;
    } finally {
      operation.unlock();
    }
  }

  public void retrust(Profile p) throws IOException {
    begin();
    try {
      writable();
      p.trustedSigningKey = null;
      issuedPlans.remove(p.id);
      saveProfiles();
    } finally {
      operation.unlock();
    }
  }

  public void optional(Profile p, String id, boolean selected) throws IOException {
    begin();
    try {
      writable();
      p.optionalSelections.put(id, selected);
      issuedPlans.remove(p.id);
      saveProfiles();
    } finally {
      operation.unlock();
    }
  }

  public Status status(Profile p, Plan plan) {
    if (readOnly) return Status.READ_ONLY;
    if (recoveryRequired()) return Status.RECOVERY_REQUIRED;
    if (restartRequired) return Status.RESTART_REQUIRED;
    if (plan == null) return Status.UNCHECKED;
    if (plan.blocked()) return Status.ERROR;
    if (plan.changes() || !p.id.equals(profiles.activeProfileId)) return Status.UPDATE_AVAILABLE;
    return Status.READY;
  }

  public void apply(Plan plan, boolean confirmedUserMods, Consumer<String> progress)
      throws Exception {
    begin();
    try {
      writable();
      if (recoveryRequired()) throw new IOException("RECOVERY_REQUIRED");
      if (issuedPlans.get(plan.profile().id) != plan) throw new IOException("CHECK_REQUIRED");
      if (plan.blocked() || (plan.confirmationRequired() && !confirmedUserMods))
        throw new IOException("CONFIRMATION_REQUIRED");
      if (!Scanner.snapshot(Scanner.scan(mods, state, selfOrigins)).equals(plan.diskSnapshot()))
        throw new IOException("PLAN_STALE");
      Map<String, Path> staged = new ConcurrentHashMap<>();
      List<Mod> installs =
          plan.items().stream()
              .filter(i -> i.local() == null && i.desired() != null && i.action() != Action.BLOCKED)
              .map(Item::desired)
              .toList();
      try (ExecutorService pool = Executors.newFixedThreadPool(4)) {
        List<Future<?>> tasks = new ArrayList<>();
        for (Mod mod : installs) {
          tasks.add(
              pool.submit(
                  () -> {
                    Artifact a = plan.manifest().artifacts.get(mod.artifact);
                    Path file =
                        network.download(
                            DiscoveryUrl.parse(plan.profile().baseUrl)
                                .base()
                                .resolve(a.download.path),
                            plan.profile().accessToken,
                            a,
                            mod.artifact,
                            root.resolve("staging"));
                    Metadata meta = Scanner.metadata(file);
                    if (!meta.id.equals(mod.modId)
                        || !meta.version.equals(mod.version)
                        || Models.SELF.equals(meta.id)
                        || "server".equals(meta.environment))
                      throw new IOException("ARTIFACT_METADATA_MISMATCH");
                    staged.put(mod.artifact, file);
                    progress.accept("download:" + staged.size() + "/" + installs.size());
                    return null;
                  }));
        }
        try {
          for (Future<?> task : tasks) task.get();
        } catch (Exception e) {
          tasks.forEach(t -> t.cancel(true));
          throw e;
        }
      }
      if (!Scanner.snapshot(Scanner.scan(mods, state, selfOrigins)).equals(plan.diskSnapshot()))
        throw new IOException("PLAN_STALE");
      Journal journal = new Journal();
      journal.previousState = FilesEx.JSON.fromJson(FilesEx.JSON.toJson(state), State.class);
      journal.previousActiveProfileId = profiles.activeProfileId;
      Path quarantine =
          root.resolve("quarantine")
              .resolve(
                  Instant.now().toEpochMilli() + "-" + plan.profile().id + "-" + UUID.randomUUID());
      List<Map<String, String>> records = new ArrayList<>();
      State next = FilesEx.JSON.fromJson(FilesEx.JSON.toJson(state), State.class);
      for (Item item : plan.items()) {
        Local local = item.local();
        if (local != null && item.action() != Action.KEEP) {
          if (local.self()) throw new IOException("SELF_PROTECTED");
          Path dst = FilesEx.child(quarantine, local.path().getFileName().toString());
          journal.moves.add(new Move(local.path(), dst, local.sha256(), false));
          records.add(
              Map.of(
                  "reason",
                  item.reason(),
                  "originalPath",
                  local.path().toString(),
                  "sha256",
                  local.sha256(),
                  "modId",
                  local.metadata().id,
                  "version",
                  local.metadata().version,
                  "quarantinedAt",
                  Instant.now().toString(),
                  "filename",
                  dst.getFileName().toString()));
          next.managedFiles.removeIf(m -> m.filename.equals(local.path().getFileName().toString()));
        }
      }
      for (Mod mod : installs) {
        String filename =
            "mld-"
                + FilesEx.safe(mod.modId)
                + "-"
                + FilesEx.safe(mod.version)
                + "-"
                + mod.artifact.substring(0, 8)
                + ".jar";
        Path dst = FilesEx.child(mods, filename);
        if (Files.exists(dst)) throw new IOException("DESTINATION_EXISTS");
        journal.moves.add(new Move(staged.get(mod.artifact), dst, mod.artifact, true));
        Managed managed = new Managed();
        managed.profileId = plan.profile().id;
        managed.serverId = plan.profile().serverId;
        managed.modId = mod.modId;
        managed.version = mod.version;
        managed.sha256 = mod.artifact;
        managed.filename = filename;
        managed.installedAt = Instant.now().toString();
        next.managedFiles.add(managed);
      }
      for (Item item : plan.items())
        if (item.action() == Action.KEEP && item.local().isManaged())
          for (Managed managed : next.managedFiles)
            if (managed.filename.equals(item.local().path().getFileName().toString())) {
              managed.profileId = plan.profile().id;
              managed.serverId = plan.profile().serverId;
            }
      if (!journal.moves.isEmpty()) {
        FilesEx.atomic(quarantine.resolve("quarantine.json"), records);
        FilesEx.atomic(root.resolve("recovery/current.json"), journal);
        restartRequired = true;
        progress.accept("install");
        for (Move move : journal.moves)
          FilesEx.move(Path.of(move.from), Path.of(move.to), move.sha256);
      }
      FilesEx.atomic(root.resolve("state.json"), next);
      state = next;
      profiles.activeProfileId = plan.profile().id;
      saveProfiles();
      Files.deleteIfExists(root.resolve("recovery/current.json"));
      issuedPlans.clear();
      progress.accept(restartRequired ? "restart" : "ready");
    } finally {
      operation.unlock();
    }
  }

  public void rollback() throws IOException {
    begin();
    try {
      writable();
      Path file = root.resolve("recovery/current.json");
      if (!Files.exists(file)) return;
      Journal j = FilesEx.read(file, Journal.class, null);
      if (j.schemaVersion != 1 || j.previousState == null || j.moves == null)
        throw new IOException("INVALID_JOURNAL");
      for (Move m : j.moves) validateMove(m);
      List<Move> reverse = new ArrayList<>(j.moves);
      Collections.reverse(reverse);
      for (Move m : reverse) {
        Path from = Path.of(m.from), to = Path.of(m.to);
        boolean src = Files.exists(from), dst = Files.exists(to);
        if (src && dst) throw new IOException("RECOVERY_AMBIGUOUS");
        if (!src && !dst) throw new IOException("RECOVERY_FILE_MISSING");
        if (dst) {
          FilesEx.move(to, from, m.sha256);
          restartRequired = true;
        } else if (!FilesEx.hash(from).equals(m.sha256))
          throw new IOException("RECOVERY_FILE_CHANGED");
      }
      FilesEx.atomic(root.resolve("state.json"), j.previousState);
      state = j.previousState;
      profiles.activeProfileId = j.previousActiveProfileId;
      saveProfiles();
      Files.delete(file);
      issuedPlans.clear();
    } finally {
      operation.unlock();
    }
  }

  private void validateMove(Move move) throws IOException {
    Path from = Path.of(move.from).toAbsolutePath().normalize(),
        to = Path.of(move.to).toAbsolutePath().normalize();
    boolean valid =
        move.install
            ? from.getParent().equals(root.resolve("staging")) && to.getParent().equals(mods)
            : from.getParent().equals(mods) && to.startsWith(root.resolve("quarantine"));
    if (!valid
        || move.sha256 == null
        || !move.sha256.matches("[0-9a-f]{64}")
        || selfOrigins.contains(from)
        || selfOrigins.contains(to)) throw new IOException("INVALID_JOURNAL");
    for (Path p : List.of(from, to)) {
      for (Path parent = p; parent != null; parent = parent.getParent())
        if (Files.isSymbolicLink(parent)) throw new IOException("UNSAFE_PATH");
      if (Files.exists(p) && Models.SELF.equals(Scanner.metadata(p).id))
        throw new IOException("SELF_PROTECTED");
    }
  }

  public List<Path> quarantined() throws IOException {
    try (var files = Files.walk(root.resolve("quarantine"), 2)) {
      return files.filter(p -> p.toString().endsWith(".jar")).sorted().toList();
    }
  }

  public void restore(Path file) throws IOException {
    begin();
    try {
      writable();
      if (recoveryRequired()) throw new IOException("RECOVERY_REQUIRED");
      file = file.toAbsolutePath().normalize();
      if (!file.startsWith(root.resolve("quarantine"))
          || Files.isSymbolicLink(file)
          || Files.isSymbolicLink(file.getParent())) throw new IOException("UNSAFE_PATH");
      var records =
          JsonParser.parseString(Files.readString(file.getParent().resolve("quarantine.json")))
              .getAsJsonArray();
      String expected = null;
      for (var record : records) {
        var obj = record.getAsJsonObject();
        if (file.getFileName().toString().equals(obj.get("filename").getAsString()))
          expected = obj.get("sha256").getAsString();
      }
      if (expected == null || !FilesEx.hash(file).equals(expected))
        throw new IOException("QUARANTINE_FILE_CHANGED");
      Metadata meta = Scanner.metadata(file);
      if (Models.SELF.equals(meta.id)) throw new IOException("SELF_PROTECTED");
      for (Local local : Scanner.scan(mods, state, selfOrigins))
        if (local.metadata() != null && meta.id.equals(local.metadata().id))
          throw new IOException("MOD_CONFLICT");
      Path dst = FilesEx.child(mods, file.getFileName().toString());
      FilesEx.move(file, dst, expected);
      restartRequired = true;
      issuedPlans.clear();
    } finally {
      operation.unlock();
    }
  }

  public void clearCache() throws IOException {
    begin();
    try {
      writable();
      try (var files = Files.list(root.resolve("cache"))) {
        for (Path p : files.toList())
          if (Files.isRegularFile(p) && !Files.isSymbolicLink(p)) Files.delete(p);
      }
    } finally {
      operation.unlock();
    }
  }

  @Override
  public void close() throws IOException {
    network.close();
    if (modsLock != null && modsLock.isValid()) modsLock.release();
    modsLockChannel.close();
    if (lock != null && lock.isValid()) lock.release();
    lockChannel.close();
  }
}
