package constant

// WelcomeMsg is used to be sent to all providers when kwatch starts
const WelcomeMsg = ":tada: kwatch@%s just started!"

// KwatchUpdateMsg is used to notify all registered providers when a newer
// version is available
const KwatchUpdateMsg = ":tada: A newer version " +
	"<https://github.com/abahmed/kwatch/releases/tag/%[1]s|%[1]s> of Kwatch " +
	"is available! Please update to the latest version."

const (
	Footer        = "<https://github.com/abahmed/kwatch|kwatch>"
	DefaultTitle  = ":red_circle: kwatch detected a crash in pod"
	DefaultText   = "There is an issue with container in a pod!"
	DefaultLogs   = "No logs captured"
	DefaultEvents = "No events captured"
)
