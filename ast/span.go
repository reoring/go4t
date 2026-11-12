// Package ast source span accessors. End is exclusive; zero End denotes a synthesized node.
package ast

func (n *TypeName) GetPos() Pos  { return n.Pos }
func (n *TypeName) GetEnd() Pos  { return n.End }
func (n *TypeName) SetEnd(p Pos) { n.End = p }

func (n *FuncType) GetPos() Pos  { return n.Pos }
func (n *FuncType) GetEnd() Pos  { return n.End }
func (n *FuncType) SetEnd(p Pos) { n.End = p }

func (n *TupleType) GetPos() Pos  { return n.Pos }
func (n *TupleType) GetEnd() Pos  { return n.End }
func (n *TupleType) SetEnd(p Pos) { n.End = p }

func (n *Param) GetPos() Pos  { return n.Pos }
func (n *Param) GetEnd() Pos  { return n.End }
func (n *Param) SetEnd(p Pos) { n.End = p }

func (n *ClassDecl) GetPos() Pos  { return n.Pos }
func (n *ClassDecl) GetEnd() Pos  { return n.End }
func (n *ClassDecl) SetEnd(p Pos) { n.End = p }

func (n *MethodSig) GetPos() Pos  { return n.Pos }
func (n *MethodSig) GetEnd() Pos  { return n.End }
func (n *MethodSig) SetEnd(p Pos) { n.End = p }

func (n *InstanceDecl) GetPos() Pos  { return n.Pos }
func (n *InstanceDecl) GetEnd() Pos  { return n.End }
func (n *InstanceDecl) SetEnd(p Pos) { n.End = p }

func (n *Constraint) GetPos() Pos  { return n.Pos }
func (n *Constraint) GetEnd() Pos  { return n.End }
func (n *Constraint) SetEnd(p Pos) { n.End = p }

func (n *InstMethod) GetPos() Pos  { return n.Pos }
func (n *InstMethod) GetEnd() Pos  { return n.End }
func (n *InstMethod) SetEnd(p Pos) { n.End = p }

func (n *TypeDecl) GetPos() Pos  { return n.Pos }
func (n *TypeDecl) GetEnd() Pos  { return n.End }
func (n *TypeDecl) SetEnd(p Pos) { n.End = p }

func (n *Ctor) GetPos() Pos  { return n.Pos }
func (n *Ctor) GetEnd() Pos  { return n.End }
func (n *Ctor) SetEnd(p Pos) { n.End = p }

func (n *FuncDecl) GetPos() Pos  { return n.Pos }
func (n *FuncDecl) GetEnd() Pos  { return n.End }
func (n *FuncDecl) SetEnd(p Pos) { n.End = p }

func (n *Block) GetPos() Pos  { return n.Pos }
func (n *Block) GetEnd() Pos  { return n.End }
func (n *Block) SetEnd(p Pos) { n.End = p }

func (n *LetStmt) GetPos() Pos  { return n.Pos }
func (n *LetStmt) GetEnd() Pos  { return n.End }
func (n *LetStmt) SetEnd(p Pos) { n.End = p }

func (n *Arm) GetPos() Pos  { return n.Pos }
func (n *Arm) GetEnd() Pos  { return n.End }
func (n *Arm) SetEnd(p Pos) { n.End = p }

func (n *DoBind) GetPos() Pos  { return n.Pos }
func (n *DoBind) GetEnd() Pos  { return n.End }
func (n *DoBind) SetEnd(p Pos) { n.End = p }

func (n *DoLet) GetPos() Pos  { return n.Pos }
func (n *DoLet) GetEnd() Pos  { return n.End }
func (n *DoLet) SetEnd(p Pos) { n.End = p }

func (n *PCtor) GetPos() Pos  { return n.Pos }
func (n *PCtor) GetEnd() Pos  { return n.End }
func (n *PCtor) SetEnd(p Pos) { n.End = p }

func (n *PVar) GetPos() Pos  { return n.Pos }
func (n *PVar) GetEnd() Pos  { return n.End }
func (n *PVar) SetEnd(p Pos) { n.End = p }

func (n *PWild) GetPos() Pos  { return n.Pos }
func (n *PWild) GetEnd() Pos  { return n.End }
func (n *PWild) SetEnd(p Pos) { n.End = p }

func (n *PLit) GetPos() Pos  { return n.Pos }
func (n *PLit) GetEnd() Pos  { return n.End }
func (n *PLit) SetEnd(p Pos) { n.End = p }

func (n *PTuple) GetPos() Pos  { return n.Pos }
func (n *PTuple) GetEnd() Pos  { return n.End }
func (n *PTuple) SetEnd(p Pos) { n.End = p }

func (n *PRecord) GetPos() Pos  { return n.Pos }
func (n *PRecord) GetEnd() Pos  { return n.End }
func (n *PRecord) SetEnd(p Pos) { n.End = p }

func (n *PAs) GetPos() Pos  { return n.Pos }
func (n *PAs) GetEnd() Pos  { return n.End }
func (n *PAs) SetEnd(p Pos) { n.End = p }

func (n *TacticArm) GetPos() Pos  { return n.Pos }
func (n *TacticArm) GetEnd() Pos  { return n.End }
func (n *TacticArm) SetEnd(p Pos) { n.End = p }

func (n *POr) GetPos() Pos  { return n.Pos }
func (n *POr) GetEnd() Pos  { return n.End }
func (n *POr) SetEnd(p Pos) { n.End = p }
