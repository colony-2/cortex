package recipe

type SequenceData struct {
	Sequence NodeList  `yaml:"sequence,omitempty"`
	Inputs   InputMap  `yaml:"inputs,omitempty"`
	Outputs  OutputMap `yaml:"outputs,omitempty"`
}
