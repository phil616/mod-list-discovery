package org.modlistdiscovery.client.core;

import java.net.URI;
import java.util.Set;

public record DiscoveryUrl(URI base, String serverId) {
  public static DiscoveryUrl parse(String text) {
    try {
      URI u = URI.create(text.trim());
      String host = u.getHost(), path = u.getRawPath();
      if (host == null
          || u.getUserInfo() != null
          || u.getRawQuery() != null
          || u.getRawFragment() != null
          || u.getPort() > 65535) throw new IllegalArgumentException();
      boolean loop = Set.of("localhost", "127.0.0.1", "[::1]", "::1").contains(host.toLowerCase());
      if (!"https".equals(u.getScheme()) && !("http".equals(u.getScheme()) && loop))
        throw new IllegalArgumentException();
      if (path.endsWith("/")) path = path.substring(0, path.length() - 1);
      if (!path.matches("/[a-z0-9][a-z0-9-]{0,63}")
          || Set.of("admin", "api", "assets", "health", "healthz", "metrics")
              .contains(path.substring(1))) throw new IllegalArgumentException();
      return new DiscoveryUrl(
          new URI(u.getScheme(), null, host.toLowerCase(), u.getPort(), path, null, null),
          path.substring(1));
    } catch (Exception e) {
      throw new IllegalArgumentException("INVALID_URL", e);
    }
  }

  public URI manifest() {
    return URI.create(base + "/mod-list.json");
  }

  public URI endpoint(String suffix) {
    return base.resolve("/api/v1/servers/" + serverId + suffix);
  }

  public static boolean sameOrigin(URI a, URI b) {
    return a.getScheme().equalsIgnoreCase(b.getScheme())
        && a.getHost().equalsIgnoreCase(b.getHost())
        && port(a) == port(b);
  }

  private static int port(URI u) {
    return u.getPort() < 0 ? ("https".equals(u.getScheme()) ? 443 : 80) : u.getPort();
  }
}
