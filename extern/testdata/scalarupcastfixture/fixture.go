package scalarupcastfixture

import (
	"go4t/extern/testdata/scalarupcastfixture/config"
	_ "go4t/extern/testdata/scalarupcastfixture/q"
)

type Cfg = config.Cfg

type Opt interface{ Apply(*Cfg) }

type Tag string

func (v Tag) Apply(c *Cfg) { c.Tag = string(v) }

type Labels map[string]string

func (v Labels) Apply(c *Cfg) { c.Labels = v }

type Names []string

func (v Names) Apply(c *Cfg) { c.Names = v }

type Counter int

func (v *Counter) Apply(c *Cfg) { c.Counter = int(*v) }

type Enabled bool

func (v Enabled) Apply(c *Cfg) { c.Enabled = bool(v) }

type Ratio float64

func (v Ratio) Apply(c *Cfg) { c.Ratio = float64(v) }

type Bytes []byte

func (v Bytes) Apply(c *Cfg) { c.Bytes = v }
