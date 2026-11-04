package main

import (
	"path/filepath"
	"testing"
)

func TestGoImportMirrorRun(t *testing.T) {
	dir := toolsModule(t)
	toolsWrite(t, filepath.Join(dir, "main.gt"), `package main
import dyn "std/dyn"
import "std/map"
import "std/io"
import "std/conc"
import "std/ref"
import f "go:go4t/extern/testdata/mirrorfixture"
func main() IO[Unit] {
 do {
  let r = f.makeRecord()
  println(show(f.echoRecord(r) == r))
  println(show(f.pointSum(r.point)))
  pointCell <- ref.new(r.point)
  pointer <- f.echoPointer(pointCell)
  println(show(pointer == Some(r.point)))
  pointerValue <- ref.get(pointCell)
  println(show(pointerValue == r.point))
  println(show(f.echoDuration(123)))
  println(show(f.echoTime(456)))
  println(show(f.echoTime(0)))
  println(f.echoBytes("copied"))
  println(show(f.echoCount(19)))
  let m = map.mapFromList([(2, [r.point])])
  println(show(f.echoMap(m) == m))
  copied <- f.withContext(r)
  println(show(copied == r))
  text <- f.contextOnly()
  println(text)
  f.errorOnly(false)
  println(show(f.variadic("ab", [3, 4])))
  println(f.dynamic([dyn.Int(7), dyn.Str("str"), dyn.Bytes("bytes"), dyn.Bool(true), dyn.Null]))
  println(debug(f.dynamicResult()))
  println(show(f.pointSum(f.pureCallback(r.point, func(p) => f.Point{x: p.x + 1, y: p.y}))))
  point <- f.iOCallback(r.point, func(p) => ioPure(f.Point{x: p.x + 2, y: p.y}))
  println(show(f.pointSum(point)))
  f.errorCallback(func() => ioPure(unit()))
  let _ = f.voidCallback(func() => unit())
  let counter = f.newCounter()
  n <- f.counterAdd(counter, 3)
  println(show(n))
  scanned <- f.counterScan(counter, 3)
  println(debug(scanned))
  text <- f.opaqueText(f.newOpaque())
  println(text)
  failure <- io.tryIO(f.errorCallback(func() => ioFail("callback failure")))
  println(match failure { Err(e) => show(e); Ok(_) => "wrong" })
  let changed = f.mutateRecord(r)
  println(show(changed.bytes == "Xytes" && r == f.makeRecord()))
  let flattened = f.makeFlattened()
  visible <- f.flattenedVisible(flattened)
  println(show(visible == "visible"))
  let ambiguous = f.Ambiguous{l: 1, r: 2, own: 3}
  println(show(f.echoAmbiguous(ambiguous) == ambiguous))
  deadline <- conc.withTimeout(1000, f.hasDeadline())
  println(show(deadline))
  optional <- f.contextOption(false)
  println(show(optional == None))
  let values = [dyn.Float(1.5), dyn.Time(11), dyn.Time(0), dyn.List([dyn.Str("nested")]), dyn.Map(map.mapFromList([("key", dyn.Int(8))]))]
  println(show(f.echoDynamic(values) == values))
  println(show(f.decodeDynamic() == [dyn.Int(-1), dyn.Int(2), dyn.Float(1.5), dyn.Bool(true), dyn.Str("s"), dyn.Bytes("b"), dyn.Time(0), dyn.Null, dyn.Str("{3}")]))
  println(show(f.namedCallback(r.point, func(p) => p) == r.point))
  negative <- io.tryIO(f.counterScan(counter, -1))
  println(match negative { Err(e) => show(e); Ok(_) => "wrong" })
 }
}
`)
	out, stderr, status := toolsCommand(t, dir, "", "run", ".")
	const want = "true\n5\ntrue\ntrue\n123\n456\n0\ncopied\n19\ntrue\ntrue\ncontext\n9\nint64:7|string:str|[]uint8:[98 121 116 101 115]|bool:true|<nil>:<nil>\nInt(42)\n6\n7\n3\n[Int(12), Bytes(\"scan\"), Null]\nopaque\ncallback failure\ntrue\ntrue\ntrue\ntrue\ntrue\ntrue\ntrue\ntrue\ngo4t scan: negative count -1\n"
	if out != want || stderr != "" || status != 0 {
		t.Fatalf("run: stdout=%q stderr=%q status=%d", out, stderr, status)
	}
}
