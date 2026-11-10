package gen

import "strings"

// RuleProvider is a downloadable domain/IP list for mihomo rules.
type RuleProvider struct {
	ID       string
	Behavior string // domain | ipcidr | classical
	Format   string // mrs | yaml | text
	URL      string
	Path     string
	Payload  []string // for inline providers
}

const (
	meta     = "https://github.com/MetaCubeX/meta-rules-dat/raw/refs/heads/meta/geo/geosite/"
	metaIP   = "https://github.com/MetaCubeX/meta-rules-dat/raw/refs/heads/meta/geo/geoip/"
	metaRaw  = "https://github.com/MetaCubeX/meta-rules-dat/raw/meta/geo/geosite/"
	dorian   = "https://github.com/dorian6996/mihomo-rules/raw/refs/heads/main/rules/"
	pentium  = "https://github.com/PentiumB/CDN-RuleSet/raw/refs/heads/main/release/"
	iconBase = "https://raw.githubusercontent.com/Koolson/Qure/master/IconSet/Color/"
)

// Providers lists every rule set the catalog can use; the generator includes only the referenced ones.
var Providers = map[string]RuleProvider{
	"apple":           {Behavior: "domain", Format: "mrs", URL: meta + "apple.mrs", Path: "./rule-sets/apple.mrs"},
	"ai_openai":       {Behavior: "domain", Format: "mrs", URL: meta + "openai.mrs", Path: "./rule-sets/openai.mrs"},
	"ai_category":     {Behavior: "domain", Format: "mrs", URL: meta + "category-ai-!cn.mrs", Path: "./rule-sets/category-ai.mrs"},
	"playstation":     {Behavior: "domain", Format: "mrs", URL: meta + "playstation.mrs", Path: "./rule-sets/playstation.mrs"},
	"xbox":            {Behavior: "domain", Format: "mrs", URL: meta + "xbox.mrs", Path: "./rule-sets/xbox.mrs"},
	"spotify":         {Behavior: "domain", Format: "mrs", URL: meta + "spotify.mrs", Path: "./rule-sets/spotify.mrs"},
	"steam":           {Behavior: "domain", Format: "yaml", URL: meta + "steam.yaml", Path: "./rule-sets/steam.yaml"},
	"youtube":         {Behavior: "domain", Format: "yaml", URL: meta + "youtube.yaml", Path: "./rule-sets/youtube.yaml"},
	"youtubeip":       {Behavior: "ipcidr", Format: "yaml", URL: dorian + "geoip/youtube.yaml", Path: "./rule-sets/youtubeip.yaml"},
	"meta":            {Behavior: "domain", Format: "mrs", URL: meta + "meta.mrs", Path: "./rule-sets/meta.mrs"},
	"metaip":          {Behavior: "ipcidr", Format: "yaml", URL: dorian + "geoip/meta.yaml", Path: "./rule-sets/metaip.yaml"},
	"telegram":        {Behavior: "domain", Format: "mrs", URL: metaRaw + "telegram.mrs", Path: "./provider/rule-set/geo/geosite/telegram.mrs"},
	"telegramip":      {Behavior: "ipcidr", Format: "mrs", URL: metaIP + "telegram.mrs", Path: "./rule-sets/telegramip.mrs"},
	"no_russia_hosts": {Behavior: "domain", Format: "mrs", URL: "https://github.com/SaltyMonkey/mrs-parsed-data/raw/refs/heads/main/block/itdog-geoblock.mrs", Path: "./rule-sets/rugeoblock.mrs"},
	"discord":         {Behavior: "domain", Format: "mrs", URL: metaRaw + "discord.mrs", Path: "./rule-sets/discord.mrs"},
	"discord_vc": {Behavior: "classical", Payload: []string{
		"AND,((IP-CIDR,138.128.136.0/21),(NETWORK,udp),(DST-PORT,50000-51000))",
		"AND,((IP-CIDR,162.158.0.0/15),(NETWORK,udp),(DST-PORT,50000-51000))",
		"AND,((IP-CIDR,172.64.0.0/13),(NETWORK,udp),(DST-PORT,50000-51000))",
		"AND,((IP-CIDR,34.0.0.0/15),(NETWORK,udp),(DST-PORT,50000-51000))",
		"AND,((IP-CIDR,34.2.0.0/15),(NETWORK,udp),(DST-PORT,50000-51000))",
		"AND,((IP-CIDR,35.192.0.0/12),(NETWORK,udp),(DST-PORT,50000-51000))",
		"AND,((IP-CIDR,35.208.0.0/12),(NETWORK,udp),(DST-PORT,50000-51000))",
		"AND,((IP-CIDR,5.200.14.128/25),(NETWORK,udp),(DST-PORT,50000-51000))",
		"AND,((IP-CIDR,66.22.192.0/18),(NETWORK,udp),(DST-PORT,50000-51000))",
	}},
	"oracle":           {Behavior: "ipcidr", Format: "mrs", URL: pentium + "oracle.mrs", Path: "./rule-sets/oracle.mrs"},
	"ubisoft":          {Behavior: "domain", Format: "mrs", URL: metaRaw + "ubisoft.mrs", Path: "./rule-sets/ubisoft.mrs"},
	"roblox":           {Behavior: "domain", Format: "mrs", URL: metaRaw + "roblox.mrs", Path: "./rule-sets/roblox.mrs"},
	"ru_blocked":       {Behavior: "domain", Format: "mrs", URL: "https://github.com/legiz-ru/mihomo-rule-sets/raw/refs/heads/main/ru-bundle/itdoginfo-inside-russia.mrs", Path: "./rule-sets/itdoginfo-inside-russia.mrs"},
	"politic":          {Behavior: "domain", Format: "yaml", URL: dorian + "geosite/politic.yaml", Path: "./rule-sets/politic.yaml"},
	"amazon":           {Behavior: "ipcidr", Format: "mrs", URL: pentium + "amazon.mrs", Path: "./rule-sets/amazon.mrs"},
	"akamaiip":         {Behavior: "ipcidr", Format: "mrs", URL: pentium + "akamai.mrs", Path: "./rule-sets/akamaiip.mrs"},
	"akamai":           {Behavior: "ipcidr", Format: "mrs", URL: meta + "akamai.mrs", Path: "./rule-sets/akamai.mrs"},
	"cloudflare":       {Behavior: "ipcidr", Format: "mrs", URL: pentium + "cloudflare.mrs", Path: "./rule-sets/cloudflare.mrs"},
	"geosite-ru":       {Behavior: "domain", Format: "yaml", URL: "https://raw.githubusercontent.com/Davoyan/mihomo-rule-sets/main/rules/category-ru.yaml", Path: "./rule-sets/geosite-ru.yaml"},
	"geoip-ru":         {Behavior: "ipcidr", Format: "mrs", URL: "https://raw.githubusercontent.com/Davoyan/ipinfo/main/geo/geoip/ru.mrs", Path: "./rule-sets/geoip-ru.mrs"},
	"geosite-private":  {Behavior: "domain", Format: "mrs", URL: metaRaw + "private.mrs", Path: "./provider/rule-set/geosite-private.mrs"},
	"geoip-private":    {Behavior: "ipcidr", Format: "mrs", URL: "https://github.com/MetaCubeX/meta-rules-dat/raw/meta/geo/geoip/private.mrs", Path: "./provider/rule-set/geoip-private.mrs"},
	"ads_domains":      {Behavior: "domain", Format: "mrs", URL: "https://github.com/zxc-rv/ad-filter/releases/latest/download/adlist.mrs", Path: "./rule-sets/adlist.mrs"},
	"geosite-netflix":  {Behavior: "domain", Format: "mrs", URL: meta + "netflix.mrs", Path: "./rule-sets/netflix.mrs"},
	"geosite-twitch":   {Behavior: "domain", Format: "mrs", URL: meta + "twitch.mrs", Path: "./rule-sets/twitch.mrs"},
	"geosite-twitter":  {Behavior: "domain", Format: "mrs", URL: meta + "twitter.mrs", Path: "./rule-sets/twitter.mrs"},
	"geosite-tiktok":   {Behavior: "domain", Format: "mrs", URL: meta + "tiktok.mrs", Path: "./rule-sets/tiktok.mrs"},
	"geosite-whatsapp": {Behavior: "domain", Format: "mrs", URL: meta + "whatsapp.mrs", Path: "./rule-sets/whatsapp.mrs"},
	"geosite-google":   {Behavior: "domain", Format: "mrs", URL: meta + "google.mrs", Path: "./rule-sets/google.mrs"},
}

// geositeLists are MetaCubeX geosite lists used by the extended catalog
// (each exists as geo/geosite/<name>.mrs; checked against the repository).
var geositeLists = []string{
	"anthropic", "google-gemini", "perplexity", "xai", "cursor", "poe", "huggingface", "elevenlabs", "github-copilot", "bing",
	"instagram", "facebook", "messenger", "threads",
	"x", "signal", "viber", "line", "kakao",
	"linkedin", "reddit", "pinterest", "tumblr", "quora", "medium", "patreon", "imgur", "flickr", "deviantart", "pixiv", "fandom",
	"vimeo", "dailymotion", "kick",
	"disney", "hbo", "primevideo", "hulu", "apple-tvplus",
	"soundcloud", "deezer", "tidal",
	"github", "gitlab", "docker", "npmjs", "jetbrains", "hashicorp", "golang", "python", "rust", "flutter", "vercel", "netlify", "heroku", "digitalocean",
	"notion", "figma", "canva", "zoom", "slack", "dropbox", "adobe", "autodesk",
	"nvidia", "intel", "amd",
	"epicgames", "ea", "origin", "blizzard", "riot", "rockstar", "faceit", "unity", "nintendo",
	"bbc", "dw", "rferl", "meduza",
	"protonmail", "tor", "torproject", "duckduckgo",
	"rutracker", "1337x",
	"coursera", "udemy", "duolingo", "wikimedia", "archive", "imdb", "goodreads",
	"paypal", "ebay", "booking", "airbnb", "ikea", "hm", "nike",
}

func init() {
	for _, name := range geositeLists {
		Providers["gs-"+name] = RuleProvider{Behavior: "domain", Format: "mrs", URL: meta + name + ".mrs", Path: "./rule-sets/" + name + ".mrs"}
	}
	// Telegram parts that live outside the usual lists: web version, Telegraph, Fragment, CDN.
	Providers["telegram_extra"] = RuleProvider{Behavior: "classical", Payload: []string{
		"DOMAIN-SUFFIX,telegram.org", "DOMAIN-SUFFIX,web.telegram.org", "DOMAIN-SUFFIX,t.me", "DOMAIN-SUFFIX,telegram.me",
		"DOMAIN-SUFFIX,telegra.ph", "DOMAIN-SUFFIX,graph.org", "DOMAIN-SUFFIX,telesco.pe", "DOMAIN-SUFFIX,tdesktop.com",
		"DOMAIN-SUFFIX,cdn-telegram.org", "DOMAIN-SUFFIX,telegram-cdn.org", "DOMAIN-SUFFIX,fragment.com", "DOMAIN-SUFFIX,contest.com",
		"DOMAIN-SUFFIX,tg.dev", "DOMAIN-SUFFIX,telegram.dog", "DOMAIN-SUFFIX,telegram.space",
	}}
}

// gs builds rule lines for the given geosite lists.
func gs(names ...string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = "RULE-SET,gs-" + n
	}
	return out
}

// Service is one switchable block of the config: a proxy group (a tile in zashboard)
// plus the rules that send matching traffic to it.
type Service struct {
	ID        string
	Name      string
	Icon      string
	ViaVPN    bool     // default choice: VPN first, or DIRECT first
	Rules     []string // "RULE-SET,id" or raw rule bodies; the group name is appended
	NoResolve []string // rule-set IDs that need ",no-resolve"
	Order     int      // position among service rules (lower = earlier)
	Default   bool     // enabled out of the box
	Category  string   // heading in the app
}

// Services is the catalog, matching the rockblack layout the user already runs.
var Services = []Service{
	{ID: "ai", Name: "AI", Icon: iconBase + "ChatGPT.png", ViaVPN: true, Rules: append([]string{"RULE-SET,ai_openai", "RULE-SET,ai_category"}, gs("anthropic", "google-gemini", "perplexity", "xai", "cursor", "poe", "huggingface", "elevenlabs", "github-copilot", "bing")...), Order: 10, Default: true, Category: "Основное"},
	{ID: "playstation", Name: "PlayStation", Icon: iconBase + "PlayStation.png", Rules: []string{"RULE-SET,playstation"}, Order: 20, Default: true, Category: "Основное"},
	{ID: "xbox", Name: "Xbox", Icon: iconBase + "Xbox.png", Rules: []string{"RULE-SET,xbox"}, Order: 21, Default: true, Category: "Основное"},
	{ID: "spotify", Name: "Spotify", Icon: iconBase + "Spotify.png", Rules: []string{"RULE-SET,spotify"}, Order: 22, Default: true, Category: "Основное"},
	{ID: "steam", Name: "Steam", Icon: iconBase + "Steam.png", Rules: []string{"RULE-SET,steam"}, Order: 23, Default: true, Category: "Основное"},
	{ID: "youtube", Name: "YouTube", Icon: iconBase + "YouTube.png", ViaVPN: true, Rules: []string{"RULE-SET,youtube"}, Order: 30, Default: true, Category: "Основное"},
	{ID: "meta", Name: "Meta", Icon: iconBase + "Facebook.png", ViaVPN: true, Rules: append([]string{"RULE-SET,meta", "RULE-SET,metaip"}, gs("instagram", "facebook", "messenger", "threads")...), NoResolve: []string{"metaip"}, Order: 31, Default: true, Category: "Основное"},
	{ID: "telegram", Name: "Telegram", Icon: iconBase + "Telegram.png", ViaVPN: true, Rules: []string{"RULE-SET,telegram", "RULE-SET,telegram_extra", "RULE-SET,telegramip"}, NoResolve: []string{"telegramip"}, Order: 32, Default: true, Category: "Основное"},
	{ID: "discord", Name: "Discord", Icon: iconBase + "Discord.png", ViaVPN: true, Rules: []string{"RULE-SET,discord", "RULE-SET,discord_vc", "PROCESS-NAME-REGEX,(?i).*discord.*"}, Order: 33, Default: true, Category: "Основное"},
	{ID: "netflix", Name: "Netflix", Icon: iconBase + "Netflix.png", ViaVPN: true, Rules: []string{"RULE-SET,geosite-netflix"}, Order: 34, Category: "Основное"},
	{ID: "twitch", Name: "Twitch", Icon: iconBase + "Twitch.png", ViaVPN: true, Rules: []string{"RULE-SET,geosite-twitch"}, Order: 35, Category: "Основное"},
	{ID: "twitter", Name: "Twitter", Icon: iconBase + "Twitter.png", ViaVPN: true, Rules: []string{"RULE-SET,geosite-twitter", "RULE-SET,gs-x"}, Order: 36, Category: "Основное"},
	{ID: "tiktok", Name: "TikTok", Icon: iconBase + "TikTok.png", ViaVPN: true, Rules: []string{"RULE-SET,geosite-tiktok"}, Order: 37, Category: "Основное"},
	{ID: "whatsapp", Name: "WhatsApp", Icon: "https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons/png/whatsapp.png", ViaVPN: true, Rules: []string{"RULE-SET,geosite-whatsapp"}, Order: 38, Category: "Основное"},
	{ID: "messengers", Name: "Мессенджеры", Icon: iconBase + "Line.png", ViaVPN: true, Rules: gs("signal", "viber", "line", "kakao"), Order: 39, Category: "Общение"},
	{ID: "social", Name: "Соцсети", Icon: iconBase + "Area.png", ViaVPN: true, Rules: gs("linkedin", "reddit", "pinterest", "tumblr", "quora", "medium", "patreon", "imgur", "flickr", "deviantart", "pixiv", "fandom"), Order: 40, Category: "Общение"},
	{ID: "video", Name: "Видео и стримы", Icon: iconBase + "Vimeo.png", ViaVPN: true, Rules: gs("vimeo", "dailymotion", "kick"), Order: 41, Category: "Видео и музыка"},
	{ID: "cinema", Name: "Кино и сериалы", Icon: iconBase + "Disney+.png", ViaVPN: true, Rules: gs("disney", "hbo", "primevideo", "hulu", "apple-tvplus"), Order: 42, Category: "Видео и музыка"},
	{ID: "music", Name: "Музыка", Icon: iconBase + "Music_Enhance.png", ViaVPN: true, Rules: gs("soundcloud", "deezer", "tidal"), Order: 43, Category: "Видео и музыка"},
	{ID: "dev", Name: "Разработка", Icon: iconBase + "GitHub.png", ViaVPN: true, Rules: gs("github", "gitlab", "docker", "npmjs", "jetbrains", "hashicorp", "golang", "python", "rust", "flutter", "vercel", "netlify", "heroku", "digitalocean"), Order: 44, Category: "Работа"},
	{ID: "work", Name: "Работа и дизайн", Icon: iconBase + "Notion.png", ViaVPN: true, Rules: gs("notion", "figma", "canva", "zoom", "slack", "dropbox", "adobe", "autodesk"), Order: 45, Category: "Работа"},
	{ID: "drivers", Name: "Драйверы и железо", Icon: iconBase + "Server.png", ViaVPN: true, Rules: gs("nvidia", "intel", "amd"), Order: 46, Category: "Работа"},
	{ID: "games", Name: "Игры и лаунчеры", Icon: iconBase + "Epic_Games.png", Rules: gs("epicgames", "ea", "origin", "blizzard", "riot", "rockstar", "faceit", "unity", "nintendo"), Order: 47, Category: "Игры"},
	{ID: "news", Name: "Новости", Icon: iconBase + "Apple_News.png", ViaVPN: true, Rules: gs("bbc", "dw", "rferl", "meduza"), Order: 48, Category: "Прочее"},
	{ID: "privacy", Name: "Приватность", Icon: iconBase + "Lock.png", ViaVPN: true, Rules: gs("protonmail", "tor", "torproject", "duckduckgo"), Order: 49, Category: "Прочее"},
	{ID: "torrents", Name: "Торрент-трекеры", Icon: iconBase + "Download.png", ViaVPN: true, Rules: gs("rutracker", "1337x"), Order: 49, Category: "Прочее"},
	{ID: "learn", Name: "Учёба и справочники", Icon: iconBase + "Bookpedia.png", ViaVPN: true, Rules: gs("coursera", "udemy", "duolingo", "wikimedia", "archive", "imdb", "goodreads"), Order: 49, Category: "Прочее"},
	{ID: "shops", Name: "Зарубежные магазины", Icon: iconBase + "PayPal.png", ViaVPN: true, Rules: gs("paypal", "ebay", "booking", "airbnb", "ikea", "hm", "nike"), Order: 49, Category: "Прочее"},
	{ID: "blocked", Name: "Заблокировано в РФ", Icon: iconBase + "Filter.png", ViaVPN: true, Rules: []string{"RULE-SET,ru_blocked", "RULE-SET,politic"}, Order: 50, Default: true, Category: "Основное"},
	{ID: "geoblock", Name: "Недоступно из РФ", Icon: "https://upload.wikimedia.org/wikipedia/commons/e/ed/RKN_site_logo_%282013-2021%29.png", ViaVPN: true, Rules: []string{"RULE-SET,no_russia_hosts", "RULE-SET,roblox", "RULE-SET,oracle"}, NoResolve: []string{"oracle"}, Order: 51, Default: true, Category: "Основное"},
	{ID: "amazon", Name: "Amazon", Icon: iconBase + "Amazon.png", Rules: []string{"RULE-SET,amazon", "RULE-SET,akamai", "RULE-SET,akamaiip"}, NoResolve: []string{"amazon", "akamaiip"}, Order: 60, Default: true, Category: "Основное"},
	{ID: "cloudflare", Name: "Cloudflare", Icon: iconBase + "Cloudflare.png", Rules: []string{"RULE-SET,cloudflare"}, NoResolve: []string{"cloudflare"}, Order: 61, Default: true, Category: "Основное"},
}

// ServiceByID finds a catalog entry.
func ServiceByID(id string) (Service, bool) {
	for _, s := range Services {
		if s.ID == id {
			return s, true
		}
	}
	return Service{}, false
}

// DefaultServiceIDs are the services enabled for a new user.
func DefaultServiceIDs() []string {
	var ids []string
	for _, s := range Services {
		if s.Default {
			ids = append(ids, s.ID)
		}
	}
	return ids
}

// Proxy-group display order in zashboard, matching the reference layout.
var groupDisplayOrder = []string{"ai", "youtube", "amazon", "cloudflare", "meta", "spotify", "playstation", "xbox", "telegram", "steam", "discord", "netflix", "twitch", "twitter", "tiktok", "whatsapp",
	"messengers", "social", "video", "cinema", "music", "dev", "work", "drivers", "games", "news", "privacy", "torrents", "learn", "shops", "blocked", "geoblock"}

// serviceNotes describe the services whose rule lists are not self-explanatory.
var serviceNotes = map[string]string{
	"ai":         "ChatGPT, Claude, Gemini, Perplexity, Grok, Copilot, Cursor и др.",
	"meta":       "Instagram, Facebook, Messenger, Threads",
	"telegram":   "приложение, веб-версия, Telegraph, Fragment",
	"blocked":    "сайты из реестра блокировок РФ",
	"geoblock":   "сервисы, которые сами закрыли доступ из РФ",
	"amazon":     "серверы Amazon и Akamai (многие игры и сайты)",
	"cloudflare": "сайты за Cloudflare",
	"twitter":    "Twitter / X",
	"messengers": "Signal, Viber, LINE, KakaoTalk",
	"social":     "LinkedIn, Reddit, Pinterest, Tumblr, Quora, Medium, Patreon, Imgur, Flickr, DeviantArt, Pixiv, Fandom",
	"video":      "Vimeo, Dailymotion, Kick",
	"cinema":     "Disney+, HBO Max, Prime Video, Hulu, Apple TV+",
	"music":      "SoundCloud, Deezer, Tidal",
	"dev":        "GitHub, GitLab, Docker Hub, npm, JetBrains, HashiCorp, Go, Python, Rust, Flutter, Vercel, Netlify, Heroku, DigitalOcean",
	"work":       "Notion, Figma, Canva, Zoom, Slack, Dropbox, Adobe, Autodesk",
	"drivers":    "NVIDIA, Intel, AMD — драйверы и ПО",
	"games":      "Epic Games, EA, Blizzard, Riot, Rockstar, FACEIT, Unity, Nintendo",
	"news":       "BBC, DW, Радио Свобода, Медуза",
	"privacy":    "Proton Mail, Tor, DuckDuckGo",
	"torrents":   "RuTracker, 1337x (сайты; сами загрузки идут напрямую)",
	"learn":      "Coursera, Udemy, Duolingo, Википедия, Internet Archive, IMDb, Goodreads",
	"shops":      "PayPal, eBay, Booking, Airbnb, IKEA, H&M, Nike",
}

// Includes is a short "what's inside" line for the app.
func (s Service) Includes() string {
	if n, ok := serviceNotes[s.ID]; ok {
		return n
	}
	var names []string
	for _, r := range s.Rules {
		if n, ok := strings.CutPrefix(r, "RULE-SET,gs-"); ok {
			names = append(names, n)
		}
	}
	return strings.Join(names, ", ")
}
