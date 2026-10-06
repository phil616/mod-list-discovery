package org.modlistdiscovery.client.ui;

import java.nio.file.Path;
import java.util.*;
import java.util.concurrent.*;
import java.util.function.Consumer;
import net.minecraft.client.gui.GuiGraphicsExtractor;
import net.minecraft.client.gui.components.Button;
import net.minecraft.client.gui.components.EditBox;
import net.minecraft.client.gui.screens.Screen;
import net.minecraft.network.chat.Component;
import org.modlistdiscovery.client.ModListDiscoveryClient;
import org.modlistdiscovery.client.core.*;
import org.modlistdiscovery.client.core.Models.*;

public final class DiscoveryScreen extends Screen {
  private final Screen parent;
  private Engine engine;
  private Profile profile;
  private Plan plan;
  private String view = "main";
  private Component message = tr("loading");
  private boolean busy = true, userConfirmed;
  private int page;
  private int maxPage;
  private List<Path> quarantine = List.of();
  private EditBox url, code, device;
  private Future<?> task;
  private String confirmationKey;
  private Runnable confirmationAction;
  private final List<Component> lines = new ArrayList<>();

  public DiscoveryScreen(Screen parent) {
    super(tr("menu.title"));
    this.parent = parent;
    ModListDiscoveryClient.ENGINE.whenComplete(
        (value, error) ->
            minecraft.execute(
                () -> {
                  engine = value;
                  busy = false;
                  message = error == null ? Component.empty() : error(error);
                  rebuildWidgets();
                }));
  }

  private static Component tr(String key, Object... args) {
    return Component.translatable("mod-list-discovery." + key, args);
  }

  private Component error(Throwable e) {
    while ((e instanceof CompletionException || e instanceof ExecutionException)
        && e.getCause() != null) e = e.getCause();
    String code = e.getMessage();
    if (code == null) code = "ERROR";
    if (code.contains(":")) code = code.substring(0, code.indexOf(':'));
    return Component.translatableWithFallback("mod-list-discovery.error." + code, code);
  }

  private Button button(Component label, int x, int y, int width, Runnable action) {
    Button b =
        addRenderableWidget(
            Button.builder(label, bn -> action.run()).bounds(x, y, width, 20).build());
    b.active = !busy;
    return b;
  }

  private void go(String next) {
    view = next;
    page = 0;
    message = Component.empty();
    rebuildWidgets();
  }

  private <T> void work(Callable<T> operation, Consumer<T> done) {
    busy = true;
    message = tr("working");
    rebuildWidgets();
    task =
        ModListDiscoveryClient.IO_EXECUTOR.submit(
            () -> {
              try {
                T value = operation.call();
                minecraft.execute(
                    () -> {
                      busy = false;
                      message = Component.empty();
                      done.accept(value);
                      rebuildWidgets();
                    });
              } catch (Exception e) {
                minecraft.execute(
                    () -> {
                      busy = false;
                      message = error(e);
                      rebuildWidgets();
                    });
              }
            });
  }

  private void check() {
    plan = null;
    work(
        () -> engine.check(profile),
        value -> {
          plan = value;
          view = "detail";
          page = 0;
        });
  }

  private void confirm(String key, Runnable yes) {
    confirmationKey = key;
    confirmationAction = yes;
    go("confirm");
  }

  @Override
  protected void init() {
    lines.clear();
    maxPage = 0;
    int x = width / 2 - 155, y = 65;
    if (engine == null) {
      button(tr("back"), x, height - 30, 310, this::onClose);
      return;
    }
    if (busy) {
      button(
                  tr("cancel"),
                  x,
                  height - 30,
                  310,
                  () -> {
                    if (task != null) task.cancel(true);
                  })
              .active =
          true;
      return;
    }
    if (engine.readOnly) lines.add(tr("status.READ_ONLY"));
    else if (engine.recoveryRequired()) {
      lines.add(tr("status.RECOVERY_REQUIRED"));
      button(
          tr("rollback"),
          x,
          100,
          310,
          () ->
              work(
                  () -> {
                    engine.rollback();
                    return null;
                  },
                  v -> go("main")));
      button(tr("back"), x, height - 30, 310, this::onClose);
      return;
    }
    switch (view) {
      case "confirm" -> {
        lines.add(tr(confirmationKey));
        button(tr("confirm"), x, 100, 150, confirmationAction);
        button(tr("cancel"), x + 160, 100, 150, () -> go(profile == null ? "main" : "detail"));
      }
      case "main" -> {
        int per = Math.max(1, (height - 155) / 26);
        List<Profile> profiles = engine.profiles.profiles;
        page = Math.min(page, Math.max(0, (profiles.size() - 1) / per));
        for (int i = page * per; i < Math.min(profiles.size(), (page + 1) * per); i++) {
          Profile p = profiles.get(i);
          button(
              Component.literal(
                  (p.id.equals(engine.profiles.activeProfileId) ? "● " : "")
                      + p.displayName
                      + " / "
                      + p.channel
                      + " / "
                      + p.lastSeenReleaseId),
              x,
              y,
              310,
              () -> {
                profile = p;
                plan = null;
                go("detail");
              });
          y += 26;
        }
        pages(profiles.size(), per, height - 85);
        button(tr("add"), x, height - 58, 150, () -> go("add"));
        button(
            tr("quarantine"),
            x + 160,
            height - 58,
            150,
            () ->
                work(
                    engine::quarantined,
                    v -> {
                      quarantine = v;
                      go("quarantine");
                    }));
      }
      case "add" -> {
        url = addRenderableWidget(new EditBox(font, x, y, 310, 20, tr("url")));
        url.setMaxLength(2048);
        url.setHint(tr("url"));
        button(
            tr("connect"),
            x,
            y + 30,
            310,
            () -> {
              String input = url.getValue();
              work(
                  () -> engine.add(input),
                  p -> {
                    profile = p;
                    view = "detail";
                    check();
                  });
            });
      }
      case "enroll" -> {
        device = addRenderableWidget(new EditBox(font, x, y, 310, 20, tr("device")));
        device.setValue("Minecraft client");
        code = addRenderableWidget(new EditBox(font, x, y + 28, 310, 20, tr("code")));
        code.setMaxLength(256);
        code.setHint(tr("code"));
        button(
            tr("enroll"),
            x,
            y + 60,
            310,
            () -> {
              String c = code.getValue(), d = device.getValue();
              work(
                  () -> {
                    engine.enroll(profile, c, d);
                    return null;
                  },
                  v -> check());
            });
      }
      case "detail" -> {
        lines.add(
            Component.literal(
                profile.displayName + " / " + profile.channel + " / " + profile.lastSeenReleaseId));
        lines.add(tr("status." + engine.status(profile, plan)));
        y = 90;
        button(tr("check"), x, y, 150, this::check);
        button(
                    tr("sync"),
                    x + 160,
                    y,
                    150,
                    () -> {
                      userConfirmed = false;
                      go("plan");
                    })
                .active =
            plan != null && !engine.readOnly;
        button(tr("enroll"), x, y + 26, 150, () -> go("enroll"));
        button(tr("optional"), x + 160, y + 26, 150, () -> go("optional")).active = plan != null;
        button(
            tr("quarantine"),
            x,
            y + 52,
            150,
            () ->
                work(
                    engine::quarantined,
                    v -> {
                      quarantine = v;
                      go("quarantine");
                    }));
        button(tr("identity"), x + 160, y + 52, 150, () -> go("identity"));
        button(
            tr("remove"),
            x,
            y + 78,
            150,
            () ->
                confirm(
                    "confirm_remove",
                    () ->
                        work(
                            () -> {
                              engine.remove(profile);
                              return null;
                            },
                            v -> {
                              profile = null;
                              plan = null;
                              go("main");
                            })));
        button(tr("runtime"), x + 160, y + 78, 150, () -> go("runtime")).active = plan != null;
      }
      case "identity" -> {
        lines.add(Component.literal(profile.baseUrl));
        lines.add(
            Component.literal(
                profile.trustedSigningKey == null ? "—" : profile.trustedSigningKey.keyId));
        button(
            tr("retrust"),
            x,
            110,
            310,
            () ->
                confirm(
                    "confirm_retrust",
                    () ->
                        work(
                            () -> {
                              engine.retrust(profile);
                              return null;
                            },
                            v -> check())));
      }
      case "runtime" -> {
        if (plan != null) {
          var r = plan.manifest().runtime;
          lines.add(Component.literal("Minecraft: " + r.minecraft));
          lines.add(Component.literal("Fabric Loader: " + r.fabricLoader));
          lines.add(Component.literal("Java: " + r.java));
          lines.add(tr("signature_valid"));
        }
      }
      case "optional" -> {
        List<Mod> optional =
            plan.manifest().mods.stream()
                .filter(m -> m.requirement == Requirement.optional)
                .toList();
        int per = Math.max(1, (height - 130) / 26);
        for (int i = page * per; i < Math.min(optional.size(), (page + 1) * per); i++) {
          Mod mod = optional.get(i);
          boolean selected =
              profile.optionalSelections.getOrDefault(mod.modId, mod.defaultSelected);
          button(
              Component.literal((selected ? "☑ " : "☐ ") + mod.modId),
              x,
              y,
              310,
              () ->
                  work(
                      () -> {
                        engine.optional(profile, mod.modId, !selected);
                        return engine.check(profile);
                      },
                      v -> plan = v));
          y += 26;
        }
        pages(optional.size(), per, height - 60);
      }
      case "plan" -> {
        if (plan == null) {
          go("detail");
          return;
        }
        int per = Math.max(1, (height - 190) / 15);
        var items = plan.items();
        lines.add(
            tr(
                "plan.summary",
                items.stream()
                    .filter(
                        i ->
                            i.action() == Action.INSTALL
                                || i.action() == Action.UPDATE
                                || i.action() == Action.OPTIONAL_INSTALL)
                    .count(),
                items.stream().filter(i -> i.action() == Action.KEEP).count(),
                items.stream()
                    .filter(
                        i ->
                            i.action() == Action.QUARANTINE_USER
                                || i.action() == Action.QUARANTINE_MANAGED
                                || i.action() == Action.OPTIONAL_REMOVE)
                    .count(),
                items.stream().filter(i -> i.action() == Action.BLOCKED).count()));
        for (int i = page * per; i < Math.min(items.size(), (page + 1) * per); i++) {
          Item item = items.get(i);
          lines.add(
              Component.literal(
                  tr("action." + item.action()).getString()
                      + "  "
                      + item.modId()
                      + (item.local() != null ? "  " + item.local().path().getFileName() : "")));
        }
        pages(items.size(), per, height - 115);
        if (plan.confirmationRequired())
          button(
              tr(userConfirmed ? "user_confirmed" : "confirm_user"),
              x,
              height - 88,
              310,
              () -> {
                userConfirmed = !userConfirmed;
                rebuildWidgets();
              });
        button(
                    tr("apply"),
                    x,
                    height - 60,
                    310,
                    () ->
                        work(
                            () -> {
                              engine.apply(
                                  plan,
                                  userConfirmed,
                                  s ->
                                      minecraft.execute(
                                          () ->
                                              message =
                                                  s.startsWith("download:")
                                                      ? tr("downloading", s.substring(9))
                                                      : tr(s)));
                              return null;
                            },
                            v -> {
                              plan = null;
                              go(engine.restartRequired ? "restart" : "detail");
                            }))
                .active =
            !plan.blocked() && (!plan.confirmationRequired() || userConfirmed) && !engine.readOnly;
      }
      case "quarantine" -> {
        int per = Math.max(1, (height - 130) / 26);
        for (int i = page * per; i < Math.min(quarantine.size(), (page + 1) * per); i++) {
          Path file = quarantine.get(i);
          button(
              tr("restore", file.getFileName().toString()),
              x,
              y,
              310,
              () ->
                  confirm(
                      "confirm_restore",
                      () ->
                          work(
                              () -> {
                                engine.restore(file);
                                return engine.quarantined();
                              },
                              v -> {
                                quarantine = v;
                                go("restart");
                              })));
          y += 26;
        }
        pages(quarantine.size(), per, height - 60);
      }
      case "restart" -> lines.add(tr("restart_required"));
      default -> {}
    }
    button(
        tr("back"),
        x,
        height - 30,
        310,
        () -> {
          if (view.equals("main") || view.equals("restart")) onClose();
          else go("main");
        });
  }

  private void pages(int count, int per, int y) {
    maxPage = Math.max(0, (count - 1) / per);
    button(
                tr("previous"),
                width / 2 - 155,
                y,
                150,
                () -> {
                  page--;
                  rebuildWidgets();
                })
            .active =
        page > 0;
    button(
                tr("next"),
                width / 2 + 5,
                y,
                150,
                () -> {
                  page++;
                  rebuildWidgets();
                })
            .active =
        (page + 1) * per < count;
  }

  @Override
  public void extractRenderState(GuiGraphicsExtractor g, int mx, int my, float delta) {
    super.extractRenderState(g, mx, my, delta);
    g.centeredText(font, title, width / 2, 20, 0xffffffff);
    g.centeredText(font, message, width / 2, 38, 0xffffcf80);
    int y = 60;
    for (Component line : lines) {
      g.text(font, line, Math.max(8, width / 2 - 230), y, 0xffffffff);
      y += 15;
    }
  }

  @Override
  public boolean mouseScrolled(double x, double y, double horizontal, double vertical) {
    if (!busy && maxPage > 0) {
      page = Math.clamp(page + (vertical < 0 ? 1 : -1), 0, maxPage);
      rebuildWidgets();
      return true;
    }
    return super.mouseScrolled(x, y, horizontal, vertical);
  }

  @Override
  public void onClose() {
    if (busy) {
      if (task != null) task.cancel(true);
      return;
    }
    minecraft.setScreenAndShow(parent);
  }
}
