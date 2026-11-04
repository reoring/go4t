package config

type Cfg struct {
	Tag     string
	Labels  map[string]string
	Names   []string
	Counter int
	Enabled bool
	Ratio   float64
	Bytes   []byte
}
