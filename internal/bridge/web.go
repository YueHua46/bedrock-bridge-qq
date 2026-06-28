package bridge

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"time"
)

func (a *App) handleSetup(w http.ResponseWriter, r *http.Request) {
	a.render(w, "setup", map[string]any{
		"Title":   "MCQQ Bridge Setup",
		"Config":  a.cfg,
		"LocalIP": localIP(),
	})
}

func (a *App) handleStatusPage(w http.ResponseWriter, r *http.Request) {
	hb, hasHB, _ := a.opt.Store.LastHeartbeat(a.cfg.Minecraft.ServerID)
	a.render(w, "status", map[string]any{
		"Title":           "MCQQ Bridge Status",
		"Config":          a.cfg,
		"OneBotConnected": a.onebot.Connected(),
		"Heartbeat":       hb,
		"HasHeartbeat":    hasHB,
		"HeartbeatAge": func() string {
			if !hasHB {
				return "no heartbeat yet"
			}
			return time.Since(hb.UpdatedAt).Round(time.Second).String()
		}(),
	})
}

func (a *App) handlePackPage(w http.ResponseWriter, r *http.Request) {
	a.render(w, "pack", map[string]any{
		"Title":  "MCQQ Bridge Pack",
		"Config": a.cfg,
	})
}

func (a *App) handleDoctorPage(w http.ResponseWriter, r *http.Request) {
	hb, hasHB, _ := a.opt.Store.LastHeartbeat(a.cfg.Minecraft.ServerID)
	items := []map[string]any{
		{"OK": true, "Name": "配置文件", "Detail": a.opt.ConfigPath},
		{"OK": a.cfg.Minecraft.Token != "", "Name": "Minecraft Token", "Detail": "已生成"},
		{"OK": a.onebot.Connected(), "Name": "OneBot WebSocket", "Detail": a.cfg.OneBot.WSURL},
		{"OK": hasHB && time.Since(hb.UpdatedAt) < 60*time.Second, "Name": "Minecraft 心跳", "Detail": fmt.Sprintf("最后心跳：%s", hb.UpdatedAt.Local().Format(time.DateTime))},
	}
	a.render(w, "doctor", map[string]any{
		"Title": "MCQQ Bridge Doctor",
		"Items": items,
	})
}

func (a *App) render(w http.ResponseWriter, name string, data map[string]any) {
	tpl := template.Must(template.New("page").Funcs(template.FuncMap{
		"json": func(v any) template.JS {
			data, _ := json.Marshal(v)
			return template.JS(data)
		},
	}).Parse(pageTemplate))
	data["Page"] = name
	if err := tpl.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

const pageTemplate = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Title}}</title>
  <style>
    :root{font-family:Arial,"Microsoft YaHei",sans-serif;color:#17202a;background:#f5f7fb}
    body{margin:0}
    header{background:#1c2833;color:white;padding:14px 22px;display:flex;gap:18px;align-items:center}
    header strong{font-size:18px}
    nav a{color:#d6eaf8;text-decoration:none;margin-right:14px}
    main{max-width:980px;margin:24px auto;padding:0 16px}
    section{background:white;border:1px solid #d9e2ec;border-radius:8px;padding:18px;margin-bottom:16px}
    h1{font-size:24px;margin:0 0 16px}
    h2{font-size:18px;margin:0 0 12px}
    label{display:block;font-size:13px;color:#52616f;margin:12px 0 6px}
    input{box-sizing:border-box;width:100%;padding:10px 12px;border:1px solid #bcccdc;border-radius:6px;font-size:15px}
    button,.button{display:inline-block;background:#0f766e;color:white;border:0;border-radius:6px;padding:10px 14px;font-size:14px;text-decoration:none;cursor:pointer}
    button.secondary{background:#334e68}
    .grid{display:grid;grid-template-columns:1fr 1fr;gap:14px}
    .muted{color:#697386;font-size:14px}
    .ok{color:#0f766e;font-weight:700}.bad{color:#b91c1c;font-weight:700}
    pre{white-space:pre-wrap;background:#102a43;color:#e6fffa;padding:14px;border-radius:8px;overflow:auto}
    @media(max-width:720px){.grid{grid-template-columns:1fr}header{display:block}nav{margin-top:10px}}
  </style>
</head>
<body>
  <header>
    <strong>MCQQ Bridge</strong>
    <nav>
      <a href="/setup">配置</a>
      <a href="/status">状态</a>
      <a href="/pack">行为包</a>
      <a href="/doctor">诊断</a>
    </nav>
  </header>
  <main>
  {{if eq .Page "setup"}}
    <section>
      <h1>配置</h1>
      <div class="grid">
        <div><label>Bridge 访问地址</label><input id="public_url" value="{{.Config.Server.PublicURL}}"></div>
        <div><label>Minecraft 服务器 ID</label><input id="server_id" value="{{.Config.Minecraft.ServerID}}"></div>
        <div><label>QQ 群号</label><input id="group_id" value="{{.Config.QQ.GroupID}}"></div>
        <div><label>QQ 转 MC 前缀</label><input id="forward_prefix" value="{{.Config.QQ.ForwardPrefix}}"></div>
        <div><label>OneBot WebSocket</label><input id="ws_url" value="{{.Config.OneBot.WSURL}}"></div>
        <div><label>OneBot HTTP</label><input id="http_url" value="{{.Config.OneBot.HTTPURL}}"></div>
        <div><label>OneBot Access Token</label><input id="onebot_token" value="{{.Config.OneBot.AccessToken}}"></div>
        <div><label>MC Token</label><input id="mc_token" value="{{.Config.Minecraft.Token}}"></div>
      </div>
      <p class="muted">BDS 和 Bridge 不在同一台机器时，把 Bridge 访问地址改成 BDS 能访问到的 IP，例如 http://{{.LocalIP}}:{{.Config.Server.Port}}。</p>
      <button onclick="save()">保存配置</button>
      <button class="secondary" onclick="testQQ()">发送 QQ 测试消息</button>
      <span id="result" class="muted"></span>
    </section>
  {{else if eq .Page "status"}}
    <section>
      <h1>运行状态</h1>
      <p>OneBot：{{if .OneBotConnected}}<span class="ok">已连接</span>{{else}}<span class="bad">未连接</span>{{end}}</p>
      <p>Minecraft 心跳：{{if .HasHeartbeat}}<span class="ok">{{.HeartbeatAge}} 前</span>{{else}}<span class="bad">尚未收到</span>{{end}}</p>
      <p>目标群：{{.Config.QQ.GroupID}}</p>
      <p>服务器 ID：{{.Config.Minecraft.ServerID}}</p>
    </section>
  {{else if eq .Page "pack"}}
    <section>
      <h1>行为包</h1>
      <p class="muted">下载后把 mcpack 放进 BDS 世界的 behavior_packs，并在世界配置里启用该行为包。</p>
      <a class="button" href="/api/pack/download">下载行为包</a>
    </section>
    <section>
      <h2>当前内置配置</h2>
      <pre>bridgeUrl: {{.Config.Server.PublicURL}}
serverId: {{.Config.Minecraft.ServerID}}
pollIntervalTicks: {{.Config.Minecraft.PollIntervalTicks}}</pre>
    </section>
  {{else if eq .Page "doctor"}}
    <section>
      <h1>诊断</h1>
      {{range .Items}}
        <p>{{if .OK}}<span class="ok">[v]</span>{{else}}<span class="bad">[x]</span>{{end}} {{.Name}} <span class="muted">{{.Detail}}</span></p>
      {{end}}
    </section>
  {{end}}
  </main>
  <script>
    const cfg = {{json .Config}};
    async function save(){
      cfg.server.public_url = document.querySelector('#public_url').value;
      cfg.minecraft.server_id = document.querySelector('#server_id').value;
      cfg.minecraft.token = document.querySelector('#mc_token').value;
      cfg.qq.group_id = Number(document.querySelector('#group_id').value);
      cfg.qq.forward_prefix = document.querySelector('#forward_prefix').value;
      cfg.onebot.ws_url = document.querySelector('#ws_url').value;
      cfg.onebot.http_url = document.querySelector('#http_url').value;
      cfg.onebot.access_token = document.querySelector('#onebot_token').value;
      const res = await fetch('/api/setup/save',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(cfg)});
      document.querySelector('#result').textContent = res.ok ? '已保存' : await res.text();
    }
    async function testQQ(){
      const res = await fetch('/api/onebot/test',{method:'POST'});
      document.querySelector('#result').textContent = res.ok ? '测试消息已发送' : await res.text();
    }
  </script>
</body>
</html>`
