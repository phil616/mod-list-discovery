package org.modlistdiscovery.client;

import static org.junit.jupiter.api.Assertions.*;

import com.google.gson.JsonParser;
import java.io.InputStreamReader;
import java.nio.charset.StandardCharsets;
import org.junit.jupiter.api.Test;
import org.modlistdiscovery.client.core.Models;

class BuildVersionTest {
  @Test
  void protocolVersionMatchesPackagedModVersion() throws Exception {
    try (var stream = getClass().getResourceAsStream("/fabric.mod.json")) {
      assertNotNull(stream);
      var metadata =
          JsonParser.parseReader(new InputStreamReader(stream, StandardCharsets.UTF_8))
              .getAsJsonObject();
      assertEquals(Models.VERSION + "+mc26.2", metadata.get("version").getAsString());
    }
  }
}
