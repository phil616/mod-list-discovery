package org.modlistdiscovery.client.core;

import java.io.*;
import java.nio.charset.StandardCharsets;
import java.nio.file.*;
import java.util.*;
import java.util.zip.*;
import org.modlistdiscovery.client.core.Models.*;

public final class Scanner {
  private Scanner() {}

  public static List<Local> scan(Path mods, State state, Set<Path> selfOrigins) throws IOException {
    List<Local> result = new ArrayList<>();
    Files.createDirectories(mods);
    try (var files = Files.list(mods)) {
      for (Path p :
          files.filter(f -> f.getFileName().toString().endsWith(".jar")).sorted().toList()) {
        if (Files.isSymbolicLink(p)) {
          result.add(new Local(p, "", null, null, "SYMLINK_JAR", selfOrigins.contains(p)));
          continue;
        }
        String hash = FilesEx.hash(p);
        Metadata meta;
        try {
          meta = metadata(p);
        } catch (IOException e) {
          result.add(new Local(p, hash, null, null, "INVALID_JAR", selfOrigins.contains(p)));
          continue;
        }
        Managed managed =
            state.managedFiles.stream()
                .filter(
                    m ->
                        p.getFileName().toString().equals(m.filename)
                            && hash.equals(m.sha256)
                            && meta.id.equals(m.modId))
                .findFirst()
                .orElse(null);
        result.add(
            new Local(
                p,
                hash,
                meta,
                managed,
                null,
                Models.SELF.equals(meta.id) || selfOrigins.contains(p)));
      }
    }
    return result;
  }

  public static Metadata metadata(Path p) throws IOException {
    try (ZipFile zip = new ZipFile(p.toFile())) {
      var entries = zip.stream().filter(e -> e.getName().equals("fabric.mod.json")).toList();
      if (entries.size() != 1 || entries.getFirst().getSize() > 1048576)
        throw new IOException("INVALID_JAR");
      try (InputStream in = zip.getInputStream(entries.getFirst())) {
        byte[] b = in.readNBytes(1048577);
        if (b.length > 1048576) throw new IOException("INVALID_JAR");
        Metadata m = FilesEx.JSON.fromJson(new String(b, StandardCharsets.UTF_8), Metadata.class);
        if (m == null
            || m.id == null
            || !m.id.matches("[a-z][a-z0-9_-]{1,63}")
            || m.version == null) throw new IOException("INVALID_JAR");
        return m;
      }
    } catch (RuntimeException e) {
      throw new IOException("INVALID_JAR", e);
    }
  }

  public static String snapshot(List<Local> locals) {
    StringBuilder b = new StringBuilder();
    for (Local l : locals) b.append(l.path()).append(':').append(l.sha256()).append(';');
    return FilesEx.hash(b.toString().getBytes(StandardCharsets.UTF_8));
  }
}
