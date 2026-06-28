package pack

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"mcqq-bridge/internal/config"
)

func Generate(cfg config.Config) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	files := map[string]string{
		"manifest.json":               manifest(),
		"scripts/config.generated.js": generatedConfig(cfg),
		"scripts/main.js":             mainJS,
		"README.txt":                  readme,
	}
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write([]byte(body)); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func manifest() string {
	data := map[string]any{
		"format_version": 2,
		"header": map[string]any{
			"name":               "MCQQ Bridge",
			"description":        "Minecraft Bedrock to QQ bridge behavior pack",
			"uuid":               uuid(),
			"version":            []int{0, 1, 0},
			"min_engine_version": []int{1, 20, 80},
		},
		"modules": []map[string]any{
			{
				"type":        "script",
				"language":    "javascript",
				"uuid":        uuid(),
				"version":     []int{0, 1, 0},
				"entry":       "scripts/main.js",
				"description": "MCQQ Bridge script runtime",
			},
		},
		"dependencies": []map[string]any{
			{"module_name": "@minecraft/server", "version": "beta"},
			{"module_name": "@minecraft/server-net", "version": "beta"},
		},
	}
	out, _ := json.MarshalIndent(data, "", "  ")
	return string(out)
}

func generatedConfig(cfg config.Config) string {
	return fmt.Sprintf(`export const CONFIG = {
  bridgeUrl: %q,
  token: %q,
  serverId: %q,
  pollIntervalTicks: %d,
  heartbeatIntervalTicks: %d
};
`, cfg.Server.PublicURL, cfg.Minecraft.Token, cfg.Minecraft.ServerID, cfg.Minecraft.PollIntervalTicks, cfg.Minecraft.HeartbeatIntervalTicks)
}

func uuid() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("00000000-0000-4000-8000-%012d", time.Now().UnixNano()%1_000_000_000_000)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	s := hex.EncodeToString(b)
	return fmt.Sprintf("%s-%s-%s-%s-%s", s[0:8], s[8:12], s[12:16], s[16:20], s[20:32])
}

const mainJS = `import { world, system } from "@minecraft/server";
import { http, HttpRequest, HttpRequestMethod, HttpHeader } from "@minecraft/server-net";
import { CONFIG } from "./config.generated.js";

console.warn("[MCQQ Bridge] script loaded, bridgeUrl=" + CONFIG.bridgeUrl);
system.runTimeout(() => {
  world.sendMessage("§a[MCQQ Bridge] 行为包脚本已加载");
}, 40);

function traceId(prefix) {
  return prefix + "-" + Date.now() + "-" + Math.random().toString(16).slice(2);
}

async function request(path, method, body) {
  const req = new HttpRequest(CONFIG.bridgeUrl.replace(/\/$/, "") + path);
  req.method = method;
  req.headers = [
    new HttpHeader("Content-Type", "application/json"),
    new HttpHeader("Authorization", "Bearer " + CONFIG.token)
  ];
  if (body !== undefined) req.body = JSON.stringify(body);
  return http.request(req);
}

async function postEvent(type, player, message) {
  try {
    await request("/api/mc/events", HttpRequestMethod.Post, {
      server_id: CONFIG.serverId,
      type,
      trace_id: traceId("mc"),
      player: { name: player?.name || "Server", xuid: "" },
      message: message || "",
      time: Math.floor(Date.now() / 1000)
    });
  } catch (err) {
    console.warn("[MCQQ Bridge] event post failed: " + err);
  }
}

if (world.afterEvents.chatSend) {
  world.afterEvents.chatSend.subscribe((ev) => {
    const text = (ev.message || "").trim();
    if (!text || text.startsWith("[QQ]")) return;
    system.run(() => postEvent("chat", ev.sender, text));
  });
  console.warn("[MCQQ Bridge] using world.afterEvents.chatSend");
} else if (world.beforeEvents.chatSend) {
  world.beforeEvents.chatSend.subscribe((ev) => {
    const text = (ev.message || "").trim();
    if (!text || text.startsWith("[QQ]")) return;
    system.run(() => postEvent("chat", ev.sender, text));
  });
  console.warn("[MCQQ Bridge] using world.beforeEvents.chatSend");
} else {
  console.warn("[MCQQ Bridge] chatSend event is not available");
}

if (world.afterEvents.playerSpawn) {
  world.afterEvents.playerSpawn.subscribe((ev) => {
    if (!ev.initialSpawn) return;
    system.run(() => postEvent("join", ev.player, ""));
  });
} else {
  console.warn("[MCQQ Bridge] world.afterEvents.playerSpawn is not available");
}

if (world.afterEvents.playerLeave) {
  world.afterEvents.playerLeave.subscribe((ev) => {
    system.run(() => postEvent("leave", { name: ev.playerName }, ""));
  });
} else {
  console.warn("[MCQQ Bridge] world.afterEvents.playerLeave is not available");
}

system.runInterval(async () => {
  try {
    const res = await request("/api/mc/pull?server_id=" + encodeURIComponent(CONFIG.serverId), HttpRequestMethod.Get);
    if (!res || res.status < 200 || res.status >= 300) return;
    const data = JSON.parse(res.body || "{}");
    const ids = [];
    for (const msg of data.messages || []) {
      world.sendMessage(msg.text);
      ids.push(msg.id);
    }
    if (ids.length) {
      await request("/api/mc/ack", HttpRequestMethod.Post, { server_id: CONFIG.serverId, ids });
    }
  } catch (err) {
    console.warn("[MCQQ Bridge] pull failed: " + err);
  }
}, CONFIG.pollIntervalTicks);

system.runInterval(async () => {
  try {
    await request("/api/mc/heartbeat", HttpRequestMethod.Post, {
      server_id: CONFIG.serverId,
      online_players: world.getPlayers().length
    });
  } catch (err) {
    console.warn("[MCQQ Bridge] heartbeat failed: " + err);
  }
}, CONFIG.heartbeatIntervalTicks);
`

const readme = `MCQQ Bridge Behavior Pack

1. Put this pack into your Bedrock Dedicated Server world's behavior_packs directory.
2. Enable it in the world behavior pack config.
3. Make sure Beta APIs / Script APIs and server-net access are enabled for your BDS version.
4. Start mcqq-bridge before starting the world.
`
