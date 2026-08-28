package discord

import (
	"context"
	"fmt"
	"strings"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/snowflake/v2"
	"github.com/groovarr/groovarr/backend/internal/config"
	"github.com/groovarr/groovarr/backend/internal/core"
	"github.com/groovarr/groovarr/backend/internal/store"
)

var globalBot *Bot

func SetGlobalBot(b *Bot) { globalBot = b }
func GetBot() *Bot        { return globalBot }

type Bot struct {
	client       *bot.Client
	cfg         *config.Config
	reportCh    snowflake.ID
	reportFn    func(string)
}

// New creates a new Discord bot.
func New(token string, reportFn func(string)) (*Bot, error) {
	cfg := config.Load()
	b := &Bot{
		cfg:      cfg,
		reportCh: snowflake.ID(cfg.ReportChannelID),
		reportFn: reportFn,
	}

	client, err := disgo.New(token,
		bot.WithGatewayConfigOpts(
			gateway.WithIntents(
				gateway.IntentGuilds,
				gateway.IntentGuildMessages,
				gateway.IntentDirectMessages,
				gateway.IntentMessageContent,
			),
		),
		bot.WithEventListenerFunc(b.onMessageCreate),
	)
	if err != nil {
		return nil, err
	}
	b.client = client
	return b, nil
}

func (b *Bot) Start() error                   { return b.client.OpenGateway(context.TODO()) }
func (b *Bot) Stop() error                    { b.client.Close(context.TODO()); return nil }
func (b *Bot) Client() *bot.Client            { return b.client }

func (b *Bot) SendReport(channelID uint64, text string) error {
	if channelID == 0 {
		return nil
	}
	ch := snowflake.ID(channelID)
	const maxLen = 1990
	start := 0
	for start < len(text) {
		end := start + maxLen
		if end >= len(text) {
			end = len(text)
		} else {
			for end > start && text[end-1] != '\n' {
				end--
			}
			if end == start {
				end = start + maxLen
			}
		}
		_, err := b.client.Rest.CreateMessage(ch, discord.MessageCreate{Content: text[start:end]})
		if err != nil {
			return err
		}
		start = end
	}
	return nil
}

func (b *Bot) onMessageCreate(event *events.MessageCreate) {
	msg := event.Message
	if msg.Author.Bot || msg.Author.System {
		return
	}
	if !strings.HasPrefix(msg.Content, "?") {
		return
	}

	content := strings.TrimPrefix(msg.Content, "?")
	parts := strings.Fields(content)
	if len(parts) == 0 {
		return
	}
	cmd := strings.ToLower(parts[0])
	args := parts[1:]

	ctx := &CommandContext{event: event, bot: b, command: cmd, args: args}

	var reply string
	switch cmd {
	case "help":
		reply = formatHelp()
	case "status":
		reply = "✅ Groovarr is running."
	case "list":
		reply = handleList()
	case "check":
		go b.runCheck(ctx)
		reply = "🔍 Running popularity check..."
	case "prune":
		go b.runPrune(ctx)
		reply = "✂️ Running prune..."
	case "add":
		if len(args) < 1 {
			reply = "Usage: `?add <artist name>`"
		} else {
			go b.runAdd(ctx, strings.Join(args, " "))
			reply = "➕ Adding artist..."
		}
	case "remove":
		if len(args) < 1 {
			reply = "Usage: `?remove <artist name>`"
		} else {
			go b.runRemove(ctx, strings.Join(args, " "))
			reply = "🗑️ Removing artist..."
		}
	default:
		reply = fmt.Sprintf("Unknown command `%s`. Try `?help`", cmd)
	}

	if reply != "" {
		b.reply(ctx, reply)
	}
}

type CommandContext struct {
	event   *events.MessageCreate
	bot     *Bot
	command string
	args    []string
}

func (b *Bot) runCheck(ctx *CommandContext) {
	results, err := core.RunDailyCheck("", false)
	if err != nil {
		b.reply(ctx, fmt.Sprintf("❌ Check failed: %v", err))
		return
	}
	b.reply(ctx, formatCheckResults(results))
}

func (b *Bot) runPrune(ctx *CommandContext) {
	results, err := core.PruneDownloadedAlbums("", false)
	if err != nil {
		b.reply(ctx, fmt.Sprintf("❌ Prune failed: %v", err))
		return
	}
	b.reply(ctx, formatPruneResults(results))
}

func (b *Bot) runAdd(ctx *CommandContext, name string) {
	artist, err := store.ArtistGet(name)
	if err == nil && artist != nil {
		b.reply(ctx, fmt.Sprintf("⚠️ Artist `%s` is already in the watchlist.", name))
		return
	}

	deezer := core.GetArtistTrackScores(name, "")
	if len(deezer.NameScores) == 0 && len(deezer.DeezerIDScores) == 0 {
		b.reply(ctx, fmt.Sprintf("❌ Could not find `%s` on Deezer or Last.fm.", name))
		return
	}

	rootFolder := b.cfg.LidarrRootFolder
	if rootFolder == "" {
		rootFolder = "Warren's Music"
	}
	addedBy := ctx.event.Message.Author.Username
	if err := store.ArtistAdd(name, "", 0, rootFolder, addedBy); err != nil {
		b.reply(ctx, fmt.Sprintf("❌ Failed to add artist: %v", err))
		return
	}

	b.reply(ctx, fmt.Sprintf("✅ Added `%s` to watchlist (folder: %s). Will be checked on next daily run.", name, rootFolder))
}

func (b *Bot) runRemove(ctx *CommandContext, name string) {
	artist, err := store.ArtistGet(name)
	if err != nil || artist == nil {
		b.reply(ctx, fmt.Sprintf("❌ Artist `%s` not found in watchlist.", name))
		return
	}
	if err := store.ArtistDelete(name); err != nil {
		b.reply(ctx, fmt.Sprintf("❌ Failed to remove: %v", err))
		return
	}
	b.reply(ctx, fmt.Sprintf("🗑️ Removed `%s` from watchlist.", name))
}

func (b *Bot) reply(ctx *CommandContext, text string) {
	_, _ = ctx.event.Client().Rest.CreateMessage(ctx.event.ChannelID, discord.MessageCreate{Content: text})
}

// ── Formatters ─────────────────────────────────────────────────────────────

func formatHelp() string {
	return `**Groovarr Commands**

?help   — Show this help
?status — Check if Groovarr is running
?list   — List watched artists
?check  — Run daily popularity check now
?prune  — Prune below-threshold tracks
?add <artist> — Add an artist to watchlist
?remove <artist> — Remove from watchlist
`
}

func handleList() string {
	artists, err := store.ArtistList()
	if err != nil || len(artists) == 0 {
		return "📭 No artists in watchlist."
	}
	var lines []string
	for _, a := range artists {
		lines = append(lines, fmt.Sprintf("- **%s** (added by %s)", a.Name, a.AddedBy))
	}
	return fmt.Sprintf("🎵 **Watchlist** (%d artists)\n%s", len(artists), strings.Join(lines, "\n"))
}

func formatCheckResults(results []core.CheckResult) string {
	if len(results) == 0 {
		return "📭 No artists in watchlist."
	}
	var parts []string
	parts = append(parts, "🎵 **Daily Hits Check Complete**\n")
	for _, r := range results {
		if len(r.Errors) > 0 {
			parts = append(parts, fmt.Sprintf("❌ **%s**: %s", r.ArtistName, r.Errors[0]))
		} else if r.AlbumsAdded == 0 && r.NewAlbumsFound == 0 {
			parts = append(parts, fmt.Sprintf("- **%s**: no new popular releases", r.ArtistName))
		} else {
			for _, a := range r.AddedAlbums {
				parts = append(parts, fmt.Sprintf("  ✅ %s", a))
			}
		}
	}
	return strings.Join(parts, "\n")
}

func formatPruneResults(results []core.PruneResult) string {
	if len(results) == 0 {
		return "✂️ Nothing to prune."
	}
	var parts []string
	parts = append(parts, "✂️ **Prune Results**\n")
	for _, r := range results {
		if r.Error != "" {
			parts = append(parts, fmt.Sprintf("❌ %s — %s: %s", r.ArtistName, r.AlbumName, r.Error))
		} else {
			parts = append(parts, fmt.Sprintf("  ✅ %s — %s: kept %d, pruned %d",
				r.ArtistName, r.AlbumName, r.KeptTracks, r.PrunedTracks))
		}
	}
	return strings.Join(parts, "\n")
}
