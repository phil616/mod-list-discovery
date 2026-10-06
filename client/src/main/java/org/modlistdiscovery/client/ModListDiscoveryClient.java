package org.modlistdiscovery.client;

import java.nio.file.*;
import java.util.*;
import java.util.concurrent.*;
import net.fabricmc.api.ClientModInitializer;
import net.fabricmc.fabric.api.client.event.lifecycle.v1.ClientLifecycleEvents;
import net.fabricmc.fabric.api.client.screen.v1.ScreenEvents;
import net.fabricmc.fabric.api.client.screen.v1.Screens;
import net.fabricmc.loader.api.FabricLoader;
import net.minecraft.client.gui.components.Button;
import net.minecraft.client.gui.screens.TitleScreen;
import net.minecraft.network.chat.Component;
import org.modlistdiscovery.client.core.Engine;
import org.modlistdiscovery.client.ui.DiscoveryScreen;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

public final class ModListDiscoveryClient implements ClientModInitializer {
  public static final Logger LOGGER = LoggerFactory.getLogger("mod-list-discovery");
  public static final ExecutorService IO_EXECUTOR =
      Executors.newSingleThreadExecutor(
          r -> {
            Thread t = new Thread(r, "mld-io");
            t.setDaemon(true);
            return t;
          });
  public static CompletableFuture<Engine> ENGINE;

  @Override
  public void onInitializeClient() {
    FabricLoader loader = FabricLoader.getInstance();
    Path game = loader.getGameDir();
    String override = System.getProperty("fabric.modsFolder");
    Path mods = override == null ? game.resolve("mods") : game.resolve(override);
    Set<Path> self = new HashSet<>();
    loader
        .getModContainer("mod-list-discovery")
        .ifPresent(
            c -> c.getOrigin().getPaths().forEach(p -> self.add(p.toAbsolutePath().normalize())));
    String
        minecraft =
            loader
                .getModContainer("minecraft")
                .orElseThrow()
                .getMetadata()
                .getVersion()
                .getFriendlyString(),
        fabric =
            loader
                .getModContainer("fabricloader")
                .orElseThrow()
                .getMetadata()
                .getVersion()
                .getFriendlyString();
    ENGINE =
        CompletableFuture.supplyAsync(
            () -> {
              try {
                return new Engine(game, mods, self, minecraft, fabric);
              } catch (Exception e) {
                LOGGER.error("Client initialization failed: {}", e.getClass().getSimpleName());
                throw new CompletionException(e);
              }
            },
            IO_EXECUTOR);
    ScreenEvents.AFTER_INIT.register(
        (client, screen, width, height) -> {
          if (screen instanceof TitleScreen)
            Screens.getWidgets(screen)
                .add(
                    Button.builder(
                            Component.translatable("mod-list-discovery.menu.title"),
                            b -> client.setScreenAndShow(new DiscoveryScreen(screen)))
                        .bounds(8, 8, 180, 20)
                        .build());
        });
    ClientLifecycleEvents.CLIENT_STOPPING.register(
        client -> {
          IO_EXECUTOR.shutdownNow();
          ENGINE.thenAccept(
              engine -> {
                try {
                  engine.close();
                } catch (Exception e) {
                  LOGGER.error("Client shutdown failed: {}", e.getClass().getSimpleName());
                }
              });
        });
  }
}
