package diff

type Kind string

const (
	KindNetwork Kind = "network"
	KindConfig  Kind = "config"
	KindSecret  Kind = "secret"
	KindService Kind = "service"
)

type Item struct {
	Kind Kind
	Name string
}

type Plan struct {
	Creates []Item
	Updates []Item
	Deletes []Item
}

func New() *Plan { return &Plan{} }
