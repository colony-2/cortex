package recipe

type SequenceData struct {
	Sequence NodeList  `yaml:"sequence,omitempty"`
	Outputs  OutputMap `yaml:"outputs,omitempty"`
}
