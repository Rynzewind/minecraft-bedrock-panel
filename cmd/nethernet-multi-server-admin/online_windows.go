package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"nethernet-multi-server/internal/accountstore"
	"nethernet-multi-server/internal/online"
	"nethernet-multi-server/internal/service"
	"strings"
	"time"
)

type onlineCard struct {
	enabled *walk.CheckBox
	account *walk.ComboBox
	world   *walk.LineEdit
	port    *walk.NumberEdit
	status  *walk.TextLabel
}
type onlinePlayerModel struct {
	walk.TableModelBase
	rows []online.Player
}

func (m *onlinePlayerModel) RowCount() int { return len(m.rows) }
func (m *onlinePlayerModel) Value(r, c int) any {
	if r < 0 || r >= len(m.rows) {
		return ""
	}
	p := m.rows[r]
	switch c {
	case 0:
		return p.Gamertag
	case 1:
		return p.XUID
	case 2:
		return p.Permissions["creative"]
	case 3:
		return p.Permissions["survival"]
	case 4:
		return "Stored policy only — ingress blocked"
	}
	return ""
}

type onlinePage struct {
	a                                       *adminWindow
	tabs                                    *walk.TabWidget
	cards                                   map[string]*onlineCard
	settings                                online.Settings
	accounts                                []service.PublicAccount
	choices                                 []service.PublicAccount
	playerModel                             onlinePlayerModel
	players                                 *walk.TableView
	gamertag, publicHost, preferred         *walk.LineEdit
	subnets                                 *walk.TextEdit
	lookupAccount, serverFilter, typeFilter *walk.ComboBox
	creative, survival                      *walk.CheckBox
	message                                 *walk.TextLabel
	diagnostics, logs                       *walk.TextEdit
	busy                                    bool
	loaded                                  bool
}

func (a *adminWindow) onlineTab() TabPage {
	p := &onlinePage{a: a, cards: map[string]*onlineCard{}}
	a.online = p
	overview := []Widget{TextLabel{Text: "Preparation build: broadcasting is blocked. Protocol compatibility, protected remote ingress and reliable LAN locality have not been verified. Existing LAN discovery remains independent."}, Composite{Layout: HBox{}, Children: []Widget{PushButton{Text: "Refresh Online status", OnClicked: p.load}, PushButton{Text: "Open Accounts", OnClicked: func() { a.navigation.SetCurrentIndex(2) }}}}}
	for _, id := range []string{"creative", "survival"} {
		c := &onlineCard{}
		p.cards[id] = c
		overview = append(overview, GroupBox{Title: strings.Title(id), Layout: VBox{}, Children: []Widget{TextLabel{AssignTo: &c.status, Text: "Disabled — load status to inspect configuration."}, CheckBox{AssignTo: &c.enabled, Text: "Enable Online Access (requested; activation remains blocked)"}, Label{Text: "Broadcaster account (authenticate through Accounts):"}, ComboBox{AssignTo: &c.account, Model: []string{"None — add a broadcaster account in Accounts"}, CurrentIndex: 0}, Label{Text: "Advertised world name:"}, LineEdit{AssignTo: &c.world}}})
	}
	overview = append(overview, PushButton{Text: "Save Online settings", OnClicked: p.save}, VSpacer{})
	authorised := TabPage{Title: "Authorised Players", Layout: VBox{}, Children: []Widget{TextLabel{Text: "Only an authenticated Xbox profile lookup can create a new managed identity. Friendship never grants access. These policies are stored, but no remote ingress is active or verified in this build."}, TableView{AssignTo: &p.players, Model: &p.playerModel, MinSize: Size{Height: 150}, StretchFactor: 1, LastColumnStretched: true, Columns: []TableViewColumn{{Title: "Gamertag", Width: 180}, {Title: "Stable XUID", Width: 180}, {Title: "Creative", Width: 85}, {Title: "Survival", Width: 85}, {Title: "Enforcement", Width: 300}}}, Label{Text: "Gamertag to resolve:"}, LineEdit{AssignTo: &p.gamertag}, ComboBox{AssignTo: &p.lookupAccount, Model: []string{"Select an authenticated broadcaster account"}, CurrentIndex: 0}, Composite{Layout: HBox{}, Children: []Widget{CheckBox{AssignTo: &p.creative, Text: "Creative"}, CheckBox{AssignTo: &p.survival, Text: "Survival"}, HSpacer{}}}, Composite{Layout: HBox{}, Children: []Widget{PushButton{Text: "Resolve and review new player...", OnClicked: p.resolve}, PushButton{Text: "Update selected permissions", OnClicked: p.updatePlayer}, PushButton{Text: "Revoke selected access", OnClicked: p.revoke}, PushButton{Text: "Remove managed record", OnClicked: p.removePlayer}}}}}
	bar:=authorised.Children[len(authorised.Children)-1].(Composite);bar.Layout=VBox{MarginsZero:true};bar.Children=[]Widget{Composite{Layout:HBox{MarginsZero:true},Children:bar.Children[:2]},Composite{Layout:HBox{MarginsZero:true},Children:bar.Children[2:]}};authorised.Children[len(authorised.Children)-1]=bar
	connectivity := []Widget{TextLabel{Text: "Local connections: existing LAN route preferred. External connections: no active listener. Friends-entry locality is Unknown; private candidates and NAT addresses are not accepted as proof. No hairpin or relay fallback is enabled."}, Label{Text: "Public IPv4 or hostname (metadata only):"}, LineEdit{AssignTo: &p.publicHost}, Label{Text: "Preferred LAN adapter/interface (preference only; existing LAN configuration is unchanged):"}, LineEdit{AssignTo: &p.preferred}, Label{Text: "Explicit trusted local subnets (one per line; not proof of a joining client's location):"}, TextEdit{AssignTo: &p.subnets, MinSize: Size{Height: 60}, MaxSize: Size{Height: 100}}}
	for _, id := range []string{"creative", "survival"} {
		connectivity = append(connectivity, Label{Text: strings.Title(id) + " external UDP port (0 = unset; not opened):"}, NumberEdit{AssignTo: &p.cards[id].port, MinValue: 0, MaxValue: 65535, Decimals: 0})
	}
	connectivity = append(connectivity, Composite{Layout: HBox{}, Children: []Widget{PushButton{Text: "Save connectivity settings", OnClicked: p.save}, PushButton{Text: "Inspect local networking", OnClicked: p.inspect}}}, TextEdit{AssignTo: &p.diagnostics, ReadOnly: true, MinSize: Size{Height: 150}, Text: "Router forwarding must not point at an unprotected BDS backend. This build does not open firewall ports, configure a router or verify external reachability. Secure ingress and protocol compatibility are required before forwarding can be planned."})
	logs := TabPage{Title: "Logs", Layout: VBox{}, Children: []Widget{TextLabel{Text: "Configuration and policy events only. No connection, transfer or routing success is invented. No authentication secrets are logged."}, Composite{Layout: HBox{}, Children: []Widget{ComboBox{AssignTo: &p.serverFilter, Model: []string{"All servers", "creative", "survival"}, CurrentIndex: 0}, ComboBox{AssignTo: &p.typeFilter, Model: []string{"All event types", "settings", "authorization"}, CurrentIndex: 0}, PushButton{Text: "Refresh logs", OnClicked: p.loadLogs}}}, TextEdit{AssignTo: &p.logs, ReadOnly: true, MinSize: Size{Height: 180}, StretchFactor: 1}}}
	return TabPage{Title: "Online", Layout: VBox{}, Children: []Widget{TextLabel{AssignTo: &p.message, Text: "Online defaults to disabled. Servers remains the only Minecraft lifecycle controller."}, TabWidget{AssignTo: &p.tabs, StretchFactor: 1, Pages: []TabPage{scrollableAdminPage(TabPage{Title: "Overview", Layout: VBox{}, Children: overview}), scrollableAdminPage(authorised), scrollableAdminPage(TabPage{Title: "Connectivity", Layout: VBox{}, Children: connectivity}), scrollableAdminPage(logs)}}}}
}
func (p *onlinePage) call(method string, params, out any, done func()) {
	if p.busy {
		return
	}
	p.busy = true
	p.message.SetText("Working...")
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		e := p.a.client.Call(ctx, method, params, out)
		p.a.sync(func() {
			p.busy = false
			if e != nil {
				p.message.SetText(e.Error())
				return
			}
			p.message.SetText("Saved/checked. Remote activation remains blocked in this preparation build.")
			if done != nil {
				done()
			}
		})
	}()
}
func (p *onlinePage) load() {
	if p.busy {
		return
	}
	p.busy = true
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		var settings online.Settings
		var state service.Status
		var statuses []online.Status
		var players []online.Player
		e := p.a.client.Call(ctx, "online.settings.get", struct{}{}, &settings)
		if e == nil {
			e = p.a.client.Call(ctx, "status.list", struct{}{}, &state)
		}
		if e == nil {
			e = p.a.client.Call(ctx, "online.status", struct{}{}, &statuses)
		}
		if e == nil {
			e = p.a.client.Call(ctx, "online.players.list", struct{}{}, &players)
		}
		p.a.sync(func() {
			p.busy = false
			if e != nil {
				p.message.SetText(e.Error())
				return
			}
			p.settings = settings
			p.accounts = state.Accounts
			p.choices = nil
			names := []string{"None"}
			for _, a := range state.Accounts {
				if accountstore.EffectiveRole(a.Role) == accountstore.RoleBroadcaster {
					p.choices = append(p.choices, a)
					names = append(names, a.PersonLabel+" / "+a.Gamertag)
				}
			}
			for id, c := range p.cards {
				s := settings.Servers[id]
				c.enabled.SetChecked(s.Enabled)
				c.world.SetText(s.WorldName)
				c.port.SetValue(float64(s.ExternalPort))
				c.account.SetModel(names)
				index := 0
				for i, a := range p.choices {
					if a.ID == s.AccountID {
						index = i + 1
					}
				}
				c.account.SetCurrentIndex(index)
			}
			p.lookupAccount.SetModel(names)
			p.lookupAccount.SetCurrentIndex(0)
			p.publicHost.SetText(settings.PublicHost)
			p.preferred.SetText(settings.PreferredInterface)
			p.subnets.SetText(strings.Join(settings.TrustedSubnets, "\r\n"))
			p.playerModel.rows = players
			p.playerModel.PublishRowsReset()
			p.applyStatuses(statuses)
			p.loaded = true
			p.message.SetText("Configuration loaded. Broadcasting, remote entry and LAN-first Friends routing are not verified or active.")
		})
	}()
}
func (p *onlinePage) applyStatuses(statuses []online.Status) {
	for _, s := range statuses {
		if c := p.cards[s.ServerID]; c != nil {
			c.status.SetText(fmt.Sprintf("%s | Server: %s | Known server players: %d\nAccount: %s | Local target: %s | Public target: %s\n%s\n%s", s.State, s.ServerState, s.Players, s.Gamertag, s.LocalTarget, s.PublicTarget, s.Routing, s.Message))
		}
	}
}
func (p *onlinePage) save() {
	if !p.loaded {
		p.message.SetText("Load Online settings before saving.")
		return
	}
	settings := p.settings
	settings.Servers = map[string]online.ServerSettings{}
	settings.PublicHost = p.publicHost.Text()
	settings.PreferredInterface = p.preferred.Text()
	settings.TrustedSubnets = strings.Fields(p.subnets.Text())
	for id, c := range p.cards {
		account := ""
		i := c.account.CurrentIndex()
		if i > 0 && i <= len(p.choices) {
			account = p.choices[i-1].ID
		}
		settings.Servers[id] = online.ServerSettings{Enabled: c.enabled.Checked(), AccountID: account, WorldName: c.world.Text(), ExternalPort: int(c.port.Value())}
	}
	p.call("online.settings.save", map[string]any{"settings": settings}, nil, p.load)
}
func (p *onlinePage) permissions() []string {
	var ids []string
	if p.creative.Checked() {
		ids = append(ids, "creative")
	}
	if p.survival.Checked() {
		ids = append(ids, "survival")
	}
	return ids
}
func (p *onlinePage) resolve() {
	i := p.lookupAccount.CurrentIndex()
	if i <= 0 || i > len(p.choices) {
		p.message.SetText("Select an authenticated broadcaster account. Sign in only through Accounts.")
		return
	}
	servers := p.permissions()
	if len(servers) == 0 {
		p.message.SetText("Select at least one server permission.")
		return
	}
	var result struct {
		ResolutionID string        `json:"resolution_id"`
		Player       online.Player `json:"player"`
	}
	p.call("online.players.resolve", map[string]string{"gamertag": p.gamertag.Text(), "account_id": p.choices[i-1].ID}, &result, func() {
		if walk.MsgBox(p.a.mw, "Review resolved remote identity", fmt.Sprintf("Store access policy for %s\nAuthenticated Xbox profile XUID: %s\nServers: %s\n\nNo remote ingress is active in this build.", result.Player.Gamertag, result.Player.XUID, strings.Join(servers, ", ")), walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) != walk.DlgCmdYes {
			return
		}
		p.call("online.players.grant", map[string]any{"resolution_id": result.ResolutionID, "servers": servers}, nil, p.load)
	})
}
func (p *onlinePage) selectedPlayer() (online.Player, bool) {
	i := p.players.CurrentIndex()
	if i < 0 || i >= len(p.playerModel.rows) {
		p.message.SetText("Select a managed player.")
		return online.Player{}, false
	}
	return p.playerModel.rows[i], true
}
func (p *onlinePage) updatePlayer() {
	v, ok := p.selectedPlayer()
	if !ok {
		return
	}
	p.call("online.players.update", map[string]any{"xuid": v.XUID, "servers": p.permissions()}, nil, p.load)
}
func (p *onlinePage) revoke() {
	v, ok := p.selectedPlayer()
	if !ok {
		return
	}
	p.call("online.players.update", map[string]any{"xuid": v.XUID, "servers": []string{}}, nil, p.load)
}
func (p *onlinePage) removePlayer() {
	v, ok := p.selectedPlayer()
	if !ok {
		return
	}
	if walk.MsgBox(p.a.mw, "Remove managed policy", "Remove this feature's player record? Existing BDS allowlists and LAN mappings are preserved.", walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) != walk.DlgCmdYes {
		return
	}
	p.call("online.players.remove", map[string]string{"xuid": v.XUID}, nil, p.load)
}
func (p *onlinePage) inspect() {
	var out any
	p.call("online.diagnostics", struct{}{}, &out, func() { b, _ := json.MarshalIndent(out, "", "  "); p.diagnostics.SetText(string(b)) })
}
func (p *onlinePage) loadLogs() {
	server, kind := "", ""
	if p.serverFilter.CurrentIndex() > 0 {
		server = p.serverFilter.Text()
	}
	if p.typeFilter.CurrentIndex() > 0 {
		kind = p.typeFilter.Text()
	}
	var events []online.Event
	p.call("online.logs", map[string]string{"server": server, "type": kind}, &events, func() {
		var b strings.Builder
		for _, e := range events {
			fmt.Fprintf(&b, "%s %s [%s] %s\r\n", e.Time.Local().Format("02 Jan 15:04:05"), e.ServerID, e.Type, e.Message)
		}
		p.logs.SetText(b.String())
	})
}

