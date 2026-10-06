package org.modlistdiscovery.client.core;

import com.google.gson.*;
import java.io.*;
import java.nio.ByteBuffer;
import java.nio.channels.FileChannel;
import java.nio.file.*;
import java.nio.file.attribute.PosixFilePermissions;
import java.security.*;
import java.util.*;

public final class FilesEx {
  public static final Gson JSON = new GsonBuilder().setPrettyPrinting().create();

  private FilesEx() {}

  public static String hash(Path p) throws IOException {
    try (InputStream in = Files.newInputStream(p)) {
      MessageDigest md = MessageDigest.getInstance("SHA-256");
      byte[] b = new byte[65536];
      for (int n; (n = in.read(b)) != -1; ) {
        if (Thread.currentThread().isInterrupted()) throw new InterruptedIOException();
        md.update(b, 0, n);
      }
      return HexFormat.of().formatHex(md.digest());
    } catch (NoSuchAlgorithmException e) {
      throw new IllegalStateException(e);
    }
  }

  public static String hash(byte[] b) {
    try {
      return HexFormat.of().formatHex(MessageDigest.getInstance("SHA-256").digest(b));
    } catch (NoSuchAlgorithmException e) {
      throw new IllegalStateException(e);
    }
  }

  public static void atomic(Path path, Object value) throws IOException {
    Files.createDirectories(path.getParent());
    Path temp = Files.createTempFile(path.getParent(), ".mld-", ".tmp");
    try {
      privateFile(temp);
      byte[] bytes = JSON.toJson(value).getBytes(java.nio.charset.StandardCharsets.UTF_8);
      try (FileChannel c = FileChannel.open(temp, StandardOpenOption.WRITE)) {
        ByteBuffer buf = ByteBuffer.wrap(bytes);
        while (buf.hasRemaining()) c.write(buf);
        c.force(true);
      }
      Files.move(temp, path, StandardCopyOption.ATOMIC_MOVE, StandardCopyOption.REPLACE_EXISTING);
    } finally {
      Files.deleteIfExists(temp);
    }
  }

  public static void privateFile(Path p) throws IOException {
    if (Files.getFileStore(p).supportsFileAttributeView("posix"))
      Files.setPosixFilePermissions(p, PosixFilePermissions.fromString("rw-------"));
  }

  public static <T> T read(Path p, Class<T> type, T defaultValue) throws IOException {
    if (!Files.exists(p)) return defaultValue;
    if (Files.size(p) > 16L * 1024 * 1024)
      throw new IOException("CORRUPTED_STATE: " + p.getFileName());
    try (Reader r = Files.newBufferedReader(p)) {
      T value = JSON.fromJson(r, type);
      if (value == null) throw new JsonParseException("null");
      return value;
    } catch (JsonParseException | IllegalStateException e) {
      throw new IOException("CORRUPTED_STATE: " + p.getFileName() + "; original retained", e);
    }
  }

  public static Path child(Path root, String name) throws IOException {
    if (name == null
        || name.contains("/")
        || name.contains("\\")
        || name.equals(".")
        || name.equals("..")) throw new IOException("UNSAFE_PATH");
    Path p = root.resolve(name).normalize();
    if (!p.startsWith(root.normalize()) || Files.isSymbolicLink(p))
      throw new IOException("UNSAFE_PATH");
    return p;
  }

  public static void move(Path from, Path to, String hash) throws IOException {
    if (Files.exists(to, LinkOption.NOFOLLOW_LINKS)) throw new IOException("DESTINATION_EXISTS");
    if (Files.isSymbolicLink(from) || !hash(from).equals(hash))
      throw new IOException("FILE_CHANGED");
    Files.createDirectories(to.getParent());
    try {
      Files.move(from, to, StandardCopyOption.ATOMIC_MOVE);
    } catch (AtomicMoveNotSupportedException e) {
      Path temp = Files.createTempFile(to.getParent(), ".mld-copy-", ".tmp");
      try {
        Files.copy(from, temp, StandardCopyOption.REPLACE_EXISTING);
        try (FileChannel c = FileChannel.open(temp, StandardOpenOption.WRITE)) {
          c.force(true);
        }
        if (!hash(temp).equals(hash)) throw new IOException("HASH_MISMATCH");
        Files.move(temp, to, StandardCopyOption.ATOMIC_MOVE);
        Files.delete(from);
      } finally {
        Files.deleteIfExists(temp);
      }
    }
  }

  public static String safe(String s) {
    String sanitized = s.replaceAll("[^a-zA-Z0-9._+-]", "_");
    return sanitized.substring(0, Math.min(sanitized.length(), 64));
  }
}
