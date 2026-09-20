package provider

type Model struct {
	ID         string       `json:"id"`
	ProviderID string       `json:"providerID"`
	Name       string       `json:"name"`
	Family     string       `json:"family,omitempty"`
	API        ModelAPI     `json:"api"`
	Status     string       `json:"status"`
	Headers    map[string]string `json:"headers"`
	Options    map[string]any    `json:"options"`
	Cost       ModelCost    `json:"cost"`
	Limit      ModelLimit   `json:"limit"`
	Capabilities ModelCaps  `json:"capabilities"`
	ReleaseDate  string     `json:"release_date,omitempty"`
	Variants     map[string]map[string]any `json:"variants,omitempty"`
}

type ModelAPI struct {
	ID  string `json:"id"`
	URL string `json:"url"`
	NPM string `json:"npm"`
}

type ModelCost struct {
	Input  float64    `json:"input"`
	Output float64    `json:"output"`
	Cache  CacheCost  `json:"cache"`
}

type CacheCost struct {
	Read  float64 `json:"read"`
	Write float64 `json:"write"`
}

type ModelLimit struct {
	Context int `json:"context"`
	Output  int `json:"output"`
}

type ModelCaps struct {
	Temperature bool      `json:"temperature"`
	Reasoning   bool      `json:"reasoning"`
	Attachment  bool      `json:"attachment"`
	ToolCall    bool      `json:"toolcall"`
	Input       ModalityCaps `json:"input"`
	Output      ModalityCaps `json:"output"`
	Interleaved bool      `json:"interleaved"`
}

type ModalityCaps struct {
	Text  bool `json:"text"`
	Audio bool `json:"audio"`
	Image bool `json:"image"`
	Video bool `json:"video"`
	PDF   bool `json:"pdf"`
}

// SizeB extracts the model size in billions of parameters from the name.
// Returns nil if not determinable.
func (m *Model) SizeB() *float64 {
	return parseModelSize(m.Name)
}
