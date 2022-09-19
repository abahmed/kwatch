package constant

// Version is the current versions of kwatch
const Version = "dev"

// WelcomeMsg is used to be sent to all providers when kwatch starts
const WelcomeMsg = ":tada: kwatch@%s just started!"

// KwatchUpdateMsg is used to notify all registered providers when a newer
// version is available
const KwatchUpdateMsg = ":tada: A newer version " +
	"<https://github.com/abahmed/kwatch/releases/tag/%[1]s|%[1]s> of Kwatch " +
	"is available! Please update to the latest version."

// NumRequeues indicates number of retries when worker fails to handle item
const NumRequeues = 5

// NumWorkers is the number concurrent workers that consume items for the queue
const NumWorkers = 4
