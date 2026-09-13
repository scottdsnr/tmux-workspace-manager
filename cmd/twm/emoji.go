package main

var emoji = map[string]string{
	"info":     "ℹ️  ",
	"list":     "📋 ",
	"active":   "🟢 ",
	"inactive": "⚪ ",
	"error":    "❌ ",
	"edit":     "📝 ",
	"warn":     "⚠️  ",
	"rocket":   "🚀 ",
	"package":  "📦 ",
	"trash":    "🗑️  ",
	"plus":     "➕ ",
	"broom":    "🧹 ",
	"check":    "✅ ",
	"wave":     "🔄 ",
	"stop":     "🛑 ",
	"sparkle":  "✨ ",
	"tool":     "🛠️  ",
	"label":    "🏷️  ",
	"folder":   "📂 ",
	"arrow":    "↳ ",
	"dry":      "🧪 ",
}

func sym(key string) string {
	if !settings.UseEmoji {
		return ""
	}
	return emoji[key]
}
