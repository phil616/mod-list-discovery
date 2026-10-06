package org.modlistdiscovery.client.core;

import java.io.*;
import java.net.*;
import java.net.http.*;
import java.nio.ByteBuffer;
import java.nio.file.*;
import java.security.*;
import java.time.Duration;
import java.util.*;
import java.util.concurrent.*;
import java.util.concurrent.Flow;
import org.modlistdiscovery.client.core.Models.*;

/** Bounded subscribers keep both response size and whole-body deadline enforceable. */
public final class Network implements AutoCloseable {
  private final HttpClient client =
      HttpClient.newBuilder()
          .connectTimeout(Duration.ofSeconds(10))
          .followRedirects(HttpClient.Redirect.NEVER)
          .build();
  private final Duration downloadTimeout;

  public Network() {
    this(Duration.ofMinutes(5));
  }

  public Network(Duration timeout) {
    downloadTimeout = timeout;
  }

  record Transfer(Path path, String sha256, long size) {}

  static final class Sink implements HttpResponse.BodySubscriber<Transfer> {
    private final CompletableFuture<Transfer> result = new CompletableFuture<>();
    private final Path path;
    private final OutputStream out;
    private long limit;
    private long count;
    private final MessageDigest digest;
    private Flow.Subscription subscription;

    Sink(Path path, long limit) throws IOException {
      this.path = path;
      this.limit = limit;
      try {
        digest = MessageDigest.getInstance("SHA-256");
      } catch (NoSuchAlgorithmException e) {
        throw new IllegalStateException(e);
      }
      out =
          Files.newOutputStream(
              path, StandardOpenOption.CREATE, StandardOpenOption.TRUNCATE_EXISTING);
    }

    public CompletionStage<Transfer> getBody() {
      return result;
    }

    public void onSubscribe(Flow.Subscription s) {
      subscription = s;
      s.request(1);
    }

    public void onNext(List<ByteBuffer> buffers) {
      try {
        for (ByteBuffer b : buffers) {
          count += b.remaining();
          if (count > limit) throw new IOException("RESPONSE_TOO_LARGE");
          byte[] data = new byte[Math.min(65536, b.remaining())];
          while (b.hasRemaining()) {
            int n = Math.min(data.length, b.remaining());
            b.get(data, 0, n);
            out.write(data, 0, n);
            digest.update(data, 0, n);
          }
        }
        subscription.request(1);
      } catch (Exception e) {
        subscription.cancel();
        onError(e);
      }
    }

    public void onError(Throwable t) {
      try {
        out.close();
      } catch (IOException e) {
        t.addSuppressed(e);
      }
      result.completeExceptionally(t);
    }

    public void onComplete() {
      try {
        out.close();
        result.complete(new Transfer(path, HexFormat.of().formatHex(digest.digest()), count));
      } catch (IOException e) {
        result.completeExceptionally(e);
      }
    }

    void cancel() {
      if (subscription != null) subscription.cancel();
      onError(new IOException("TIMEOUT_OR_CANCELLED"));
    }
  }

  private HttpResponse<Transfer> request(
      URI original,
      String token,
      String method,
      String body,
      Path output,
      long limit,
      Duration timeout,
      boolean redirects)
      throws IOException, InterruptedException {
    URI uri = original;
    long deadline = System.nanoTime() + timeout.toNanos();
    for (int hop = 0; hop <= 5; hop++) {
      if (!Set.of("https", "http").contains(uri.getScheme())
          || uri.getUserInfo() != null
          || uri.getHost() == null) throw new IOException("INVALID_URL");
      if (original.getScheme().equals("https") && !uri.getScheme().equals("https"))
        throw new IOException("HTTPS_DOWNGRADE");
      long remaining = deadline - System.nanoTime();
      if (remaining <= 0) throw new IOException("TIMEOUT");
      HttpRequest.Builder b = HttpRequest.newBuilder(uri).timeout(Duration.ofNanos(remaining));
      if (token != null && DiscoveryUrl.sameOrigin(original, uri))
        b.header("Authorization", "Bearer " + token);
      if (body != null)
        b.header("Content-Type", "application/json")
            .method(method, HttpRequest.BodyPublishers.ofString(body));
      else b.GET();
      Sink sink = new Sink(output, limit);
      CompletableFuture<HttpResponse<Transfer>> future =
          client.sendAsync(
              b.build(),
              info -> {
                if (info.statusCode() != 200 && info.statusCode() != 201) sink.limit = 65536;
                return sink;
              });
      HttpResponse<Transfer> response;
      try {
        response = future.get(remaining, TimeUnit.NANOSECONDS);
      } catch (TimeoutException e) {
        future.cancel(true);
        sink.cancel();
        throw new IOException("TIMEOUT", e);
      } catch (ExecutionException e) {
        sink.cancel();
        Throwable cause = e.getCause();
        String code = "DOWNLOAD_FAILED";
        for (Throwable t = cause; t != null; t = t.getCause()) {
          if (t instanceof javax.net.ssl.SSLException) code = "TLS_ERROR";
          else if (t instanceof java.net.UnknownHostException
              || t instanceof java.nio.channels.UnresolvedAddressException) code = "DNS_ERROR";
          else if (t instanceof HttpTimeoutException) code = "TIMEOUT";
        }
        throw new IOException(code, cause);
      } catch (InterruptedException e) {
        future.cancel(true);
        sink.cancel();
        throw e;
      }
      int status = response.statusCode();
      if (Set.of(301, 302, 303, 307, 308).contains(status)) {
        if (!redirects || hop == 5) throw new IOException("REDIRECT_REJECTED");
        String location =
            response
                .headers()
                .firstValue("Location")
                .orElseThrow(() -> new IOException("INVALID_REDIRECT"));
        URI next = uri.resolve(location);
        if (uri.getScheme().equals("https") && !next.getScheme().equals("https"))
          throw new IOException("HTTPS_DOWNGRADE");
        if (!next.getScheme().equals("https")
            && !(next.getScheme().equals("http")
                && Set.of("localhost", "127.0.0.1", "[::1]").contains(next.getHost())))
          throw new IOException("INSECURE_REDIRECT");
        uri = next;
        continue;
      }
      if (status != 200 && status != 201) throw new IOException("HTTP_" + status);
      return response;
    }
    throw new IOException("REDIRECT_REJECTED");
  }

  public byte[] json(URI uri, String token, String body, Path tempDir)
      throws IOException, InterruptedException {
    Files.createDirectories(tempDir);
    Path temp = Files.createTempFile(tempDir, "http-", ".json");
    try {
      request(
          uri,
          token,
          body == null ? "GET" : "POST",
          body,
          temp,
          2 * 1024 * 1024,
          Duration.ofSeconds(30),
          false);
      return Files.readAllBytes(temp);
    } finally {
      Files.deleteIfExists(temp);
    }
  }

  public Path download(URI uri, String token, Artifact artifact, String hash, Path staging)
      throws IOException, InterruptedException {
    Files.createDirectories(staging);
    Path part = FilesEx.child(staging, hash + ".part"), jar = FilesEx.child(staging, hash + ".jar");
    if (Files.exists(jar) && Files.size(jar) == artifact.size && FilesEx.hash(jar).equals(hash))
      return jar;
    try {
      var response = request(uri, token, "GET", null, part, artifact.size, downloadTimeout, true);
      if (response.body().size() != artifact.size) throw new IOException("SIZE_MISMATCH");
      if (!response.body().sha256().equals(hash)) throw new IOException("HASH_MISMATCH");
      try (var c = java.nio.channels.FileChannel.open(part, StandardOpenOption.WRITE)) {
        c.force(true);
      }
      Files.move(part, jar, StandardCopyOption.ATOMIC_MOVE, StandardCopyOption.REPLACE_EXISTING);
      return jar;
    } finally {
      Files.deleteIfExists(part);
    }
  }

  @Override
  public void close() {
    client.close();
  }
}
