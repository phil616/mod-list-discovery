package org.modlistdiscovery.client.core;

import com.google.gson.*;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.security.*;
import java.security.spec.X509EncodedKeySpec;
import java.util.*;
import org.modlistdiscovery.client.core.Models.*;

public final class ManifestVerifier {
  public record Verified(Manifest manifest, SigningKey key) {}

  public static Verified verify(byte[] envelope, Profile profile) throws IOException {
    if (envelope.length > 2 * 1024 * 1024) throw new IOException("MANIFEST_TOO_LARGE");
    try {
      JsonObject obj =
          JsonParser.parseString(new String(envelope, StandardCharsets.UTF_8)).getAsJsonObject();
      if (obj.get("schemaVersion").getAsInt() != 1
          || !obj.get("payloadEncoding").getAsString().equals("base64url"))
        throw new IOException("UNSUPPORTED_SCHEMA");
      JsonObject sig = obj.getAsJsonObject("signature");
      if (!sig.get("algorithm").getAsString().equals("Ed25519"))
        throw new IOException("INVALID_SIGNATURE");
      byte[] raw = base64(sig.get("publicKey").getAsString()),
          payload = base64(obj.get("payload").getAsString()),
          signature = base64(sig.get("value").getAsString());
      if (raw.length != 32 || signature.length != 64) throw new IOException("INVALID_SIGNATURE");
      String keyId = "ed25519:" + FilesEx.hash(raw).substring(0, 16);
      if (!keyId.equals(sig.get("keyId").getAsString())) throw new IOException("INVALID_SIGNATURE");
      String encoded = Base64.getUrlEncoder().withoutPadding().encodeToString(raw);
      if (profile.trustedSigningKey != null
          && (!encoded.equals(profile.trustedSigningKey.publicKey)
              || !keyId.equals(profile.trustedSigningKey.keyId)))
        throw new IOException("SIGNING_KEY_CHANGED");
      byte[] prefix = HexFormat.of().parseHex("302a300506032b6570032100"),
          x509 = new byte[prefix.length + 32];
      System.arraycopy(prefix, 0, x509, 0, prefix.length);
      System.arraycopy(raw, 0, x509, prefix.length, 32);
      PublicKey key =
          KeyFactory.getInstance("Ed25519").generatePublic(new X509EncodedKeySpec(x509));
      Signature verifier = Signature.getInstance("Ed25519");
      verifier.initVerify(key);
      verifier.update(payload);
      if (!verifier.verify(signature)) throw new IOException("INVALID_SIGNATURE");
      Manifest m =
          FilesEx.JSON.fromJson(new String(payload, StandardCharsets.UTF_8), Manifest.class);
      validate(m, profile);
      SigningKey trusted = new SigningKey();
      trusted.keyId = keyId;
      trusted.publicKey = encoded;
      return new Verified(m, trusted);
    } catch (IOException e) {
      throw e;
    } catch (GeneralSecurityException | RuntimeException e) {
      throw new IOException("INVALID_SIGNATURE_OR_MANIFEST", e);
    }
  }

  private static byte[] base64(String s) {
    if (!s.matches("[A-Za-z0-9_-]+")) throw new IllegalArgumentException("invalid base64url");
    return Base64.getUrlDecoder().decode(s);
  }

  public static void validate(Manifest m, Profile p) throws IOException {
    if (m == null || m.schemaVersion != 1) throw new IOException("UNSUPPORTED_SCHEMA");
    if (m.server == null
        || !p.serverId.equals(m.server.id)
        || !p.channel.equals(m.channel)
        || m.release == null
        || m.release.id < 1
        || m.release.publishedAt == null
        || m.runtime == null
        || m.mods == null
        || m.artifacts == null
        || m.mods.size() > 500
        || m.artifacts.size() > 500) throw new IOException("INVALID_MANIFEST");
    if (m.runtime.minecraft == null || m.runtime.fabricLoader == null || m.runtime.java == null)
      throw new IOException("INVALID_MANIFEST");
    Set<String> ids = new HashSet<>();
    for (Mod mod : m.mods) {
      if (mod == null
          || mod.modId == null
          || !mod.modId.matches("[a-z][a-z0-9_-]{1,63}")
          || Models.SELF.equals(mod.modId)
          || !ids.add(mod.modId)
          || mod.requirement == null) throw new IOException("INVALID_MANIFEST");
      if (mod.requirement == Requirement.forbidden) {
        if (mod.artifact != null) throw new IOException("INVALID_MANIFEST");
      } else if (mod.artifact == null
          || !m.artifacts.containsKey(mod.artifact)
          || mod.version == null) throw new IOException("INVALID_MANIFEST");
    }
    for (var entry : m.artifacts.entrySet()) {
      String h = entry.getKey();
      Artifact a = entry.getValue();
      String expected = "/api/v1/servers/" + p.serverId + "/artifacts/" + h + "/download";
      if (!h.matches("[0-9a-f]{64}")
          || a == null
          || a.size < 1
          || a.size > 536870912L
          || a.download == null
          || !"api".equals(a.download.type)
          || !expected.equals(a.download.path)) throw new IOException("INVALID_MANIFEST");
    }
  }
}
