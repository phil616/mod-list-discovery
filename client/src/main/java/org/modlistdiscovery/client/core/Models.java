package org.modlistdiscovery.client.core;

import java.nio.file.Path;
import java.util.*;

public final class Models {
  private Models() {}

  public static final String SELF = "mod-list-discovery";
  public static final String VERSION = BuildVersion.VERSION;

  public static class SigningKey {
    public String keyId, publicKey;
  }

  public static class Profile {
    public String id = UUID.randomUUID().toString(),
        baseUrl,
        serverId,
        displayName,
        channel = "stable",
        deviceId,
        accessToken;
    public SigningKey trustedSigningKey;
    public Map<String, Boolean> optionalSelections = new HashMap<>();
    public long lastSeenReleaseId;
  }

  public static class Profiles {
    public int schemaVersion = 1;
    public String activeProfileId;
    public List<Profile> profiles = new ArrayList<>();
  }

  public static class State {
    public int schemaVersion = 1;
    public List<Managed> managedFiles = new ArrayList<>();
  }

  public static class Managed {
    public String serverId, profileId, modId, version, sha256, filename, installedAt;
  }

  public static class Manifest {
    public int schemaVersion;
    public Server server;
    public String channel;
    public Release release;
    public Runtime runtime;
    public List<Mod> mods;
    public Map<String, Artifact> artifacts;
  }

  public static class Server {
    public String id, displayName;
  }

  public static class Release {
    public long id;
    public String name, publishedAt;
  }

  public static class Runtime {
    public String minecraft, fabricLoader, java, minDiscoveryVersion;
  }

  public enum Requirement {
    required,
    optional,
    forbidden
  }

  public static class Mod {
    public String modId, name, version, artifact, versionConstraint;
    public Requirement requirement;
    public boolean defaultSelected;
  }

  public static class Artifact {
    public String filename;
    public long size;
    public Download download;
  }

  public static class Download {
    public String type, path;
  }

  public static class Metadata {
    public String id, name, version, environment;
    public Map<String, Object> depends, conflicts, breaks;
    public List<String> provides;
  }

  public record Local(
      Path path, String sha256, Metadata metadata, Managed managed, String error, boolean self) {
    public boolean isManaged() {
      return managed != null;
    }
  }

  public enum Action {
    KEEP,
    INSTALL,
    UPDATE,
    QUARANTINE_MANAGED,
    QUARANTINE_USER,
    OPTIONAL_INSTALL,
    OPTIONAL_REMOVE,
    BLOCKED
  }

  public record Item(Action action, String modId, Local local, Mod desired, String reason) {}

  public record Plan(Manifest manifest, Profile profile, List<Item> items, String diskSnapshot) {
    public boolean blocked() {
      return items.stream().anyMatch(i -> i.action() == Action.BLOCKED);
    }

    public boolean confirmationRequired() {
      return items.stream().anyMatch(i -> i.action() == Action.QUARANTINE_USER);
    }

    public boolean changes() {
      return items.stream()
          .anyMatch(i -> i.action() != Action.KEEP && i.action() != Action.BLOCKED);
    }
  }

  public enum Status {
    UNCHECKED,
    READY,
    UPDATE_AVAILABLE,
    RESTART_REQUIRED,
    RECOVERY_REQUIRED,
    READ_ONLY,
    ERROR
  }

  public static class Move {
    public String from, to, sha256;
    public boolean install;

    public Move() {}

    public Move(Path from, Path to, String hash, boolean install) {
      this.from = from.toString();
      this.to = to.toString();
      this.sha256 = hash;
      this.install = install;
    }
  }

  public static class Journal {
    public int schemaVersion = 1;
    public State previousState;
    public String previousActiveProfileId;
    public List<Move> moves = new ArrayList<>();
  }
}
