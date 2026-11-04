package q

import "github.com/reoring/go4t/extern/testdata/scalarupcastfixture/config"

type QTag string

func (v QTag) Apply(c *config.Cfg) { c.Tag = string(v) }
