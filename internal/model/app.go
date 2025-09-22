package model

const (
	AppServiceName = "media_exporter"
	NamespaceName  = "webitel"
)

var versions = []string{
	"25.10",
	"25.08",
	"25.06",
	"25.04",
	"25.02",
}

var (
	CurrentVersion = versions[0]
)
