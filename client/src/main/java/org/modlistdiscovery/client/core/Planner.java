package org.modlistdiscovery.client.core;

import java.io.IOException;
import java.util.*;
import net.fabricmc.loader.api.Version;
import net.fabricmc.loader.api.metadata.version.VersionPredicate;
import org.modlistdiscovery.client.core.Models.*;

public final class Planner {
  public static boolean matches(String constraint, String version) throws IOException {
    try {
      return VersionPredicate.parse(constraint == null ? "*" : constraint)
          .test(Version.parse(version));
    } catch (Exception e) {
      throw new IOException("INVALID_VERSION_CONSTRAINT", e);
    }
  }

  public static void runtime(Manifest m, String minecraft, String loader, String java)
      throws IOException {
    if (!m.runtime.minecraft.equals(minecraft)
        || !matches(m.runtime.fabricLoader, loader)
        || !matches(m.runtime.java, java)
        || (m.runtime.minDiscoveryVersion != null
            && !m.runtime.minDiscoveryVersion.isBlank()
            && !matches(">=" + m.runtime.minDiscoveryVersion, Models.VERSION)))
      throw new IOException("INCOMPATIBLE_RUNTIME");
  }

  public static Plan plan(Manifest m, Profile p, List<Local> locals) throws IOException {
    ManifestVerifier.validate(m, p);
    List<Item> items = new ArrayList<>();
    Set<PathKey> handled = new HashSet<>();
    for (Local l : locals)
      if (l.error() != null)
        items.add(new Item(Action.BLOCKED, l.path().getFileName().toString(), l, null, l.error()));
    for (Mod mod : m.mods) {
      List<Local> same =
          locals.stream()
              .filter(l -> l.metadata() != null && mod.modId.equals(l.metadata().id) && !l.self())
              .toList();
      boolean wanted =
          mod.requirement == Requirement.required
              || (mod.requirement == Requirement.optional
                  && p.optionalSelections.getOrDefault(mod.modId, mod.defaultSelected));
      if (mod.requirement == Requirement.forbidden) {
        for (Local l : same)
          if (matches(mod.versionConstraint, l.metadata().version)) {
            quarantine(items, l, mod, "forbidden");
            handled.add(new PathKey(l));
          }
        continue;
      }
      if (!wanted) {
        for (Local l : same) {
          handled.add(new PathKey(l));
          if (l.isManaged())
            items.add(new Item(Action.OPTIONAL_REMOVE, mod.modId, l, mod, "optional disabled"));
        }
        continue;
      }
      Local correct =
          same.stream().filter(l -> l.sha256().equals(mod.artifact)).findFirst().orElse(null);
      for (Local l : same) {
        handled.add(new PathKey(l));
        if (l == correct) {
          items.add(new Item(Action.KEEP, mod.modId, l, mod, "exact SHA256"));
        } else quarantine(items, l, mod, "same mod ID conflict");
      }
      if (correct == null)
        items.add(
            new Item(
                !same.isEmpty()
                    ? Action.UPDATE
                    : mod.requirement == Requirement.optional
                        ? Action.OPTIONAL_INSTALL
                        : Action.INSTALL,
                mod.modId,
                null,
                mod,
                "artifact required"));
    }
    for (Local l : locals)
      if (l.metadata() != null && !l.self() && !handled.contains(new PathKey(l)) && l.isManaged())
        quarantine(items, l, null, "not required by active profile");
    Set<Local> removed = new HashSet<>();
    Map<String, Integer> finalIds = new HashMap<>();
    for (Item item : items) {
      if (item.local() != null && item.action() != Action.KEEP && item.action() != Action.BLOCKED)
        removed.add(item.local());
      if (item.local() == null && item.desired() != null && item.action() != Action.BLOCKED)
        finalIds.merge(item.modId(), 1, Integer::sum);
    }
    for (Local local : locals)
      if (local.metadata() != null && !removed.contains(local))
        finalIds.merge(local.metadata().id, 1, Integer::sum);
    for (var entry : finalIds.entrySet())
      if (entry.getValue() > 1)
        items.add(new Item(Action.BLOCKED, entry.getKey(), null, null, "duplicate local mod IDs"));
    return new Plan(m, p, List.copyOf(items), Scanner.snapshot(locals));
  }

  private record PathKey(String path) {
    PathKey(Local l) {
      this(l.path().toString());
    }
  }

  private static void quarantine(List<Item> items, Local l, Mod m, String reason) {
    items.add(
        new Item(
            l.isManaged() ? Action.QUARANTINE_MANAGED : Action.QUARANTINE_USER,
            l.metadata().id,
            l,
            m,
            reason));
  }
}
