package main

import (
	"path/filepath"
	"testing"
)

func TestBindMirrorRun(t *testing.T) {
	dir := toolsModule(t)
	out, stderr, status := toolsCommand(t, dir, "", "bind", "go4t/extern/testdata/mirrorfixture", "--shim", "shim", "-o", "bindings.gt")
	if out != "" || stderr != "" || status != 0 {
		t.Fatalf("bind: stdout=%q stderr=%q status=%d", out, stderr, status)
	}
	toolsWrite(t, filepath.Join(dir, "main.gt"), `package main
import dyn "std/dyn"
import "std/map"
import "std/io"
import "std/conc"
func main() IO[Unit] {
 do {
  let r = makeRecord()
  println(show(echoRecord(r) == r))
  println(show(pointSum(r.point)))
  println(show(echoPointer(Some(r.point)) == Some(r.point)))
  println(show(echoPointer(None) == None))
  println(show(echoDuration(123)))
  println(show(echoTime(456)))
  println(show(echoTime(0)))
  println(echoBytes("copied"))
  println(show(echoCount(19)))
  let m = map.mapFromList([(2, [r.point])])
  println(show(echoMap(m) == m))
  copied <- withContext(r)
  println(show(copied == r))
  text <- contextOnly()
  println(text)
  errorOnly(false)
  println(show(variadic("ab", [3, 4])))
  println(dynamic([dyn.Int(7), dyn.Str("str"), dyn.Bytes("bytes"), dyn.Bool(true), dyn.Null]))
  println(debug(dynamicResult()))
  println(show(pointSum(pureCallback(r.point, func(p) => Point{x: p.x + 1, y: p.y}))))
  point <- iOCallback(r.point, func(p) => ioPure(Point{x: p.x + 2, y: p.y}))
  println(show(pointSum(point)))
  errorCallback(func() => ioPure(unit()))
  let _ = voidCallback(func() => unit())
  let counter = newCounter()
  n <- counterAdd(counter, 3)
  println(show(n))
  scanned <- counterScan(counter, 3)
  println(debug(scanned))
  text <- opaqueText(newOpaque())
  println(text)
  failure <- io.tryIO(errorCallback(func() => ioFail("callback failure")))
  println(match failure { Err(e) => show(e); Ok(_) => "wrong" })
  let changed = mutateRecord(r)
  println(show(changed.bytes == "Xytes" && r == makeRecord()))
  let flattened = makeFlattened()
  visible <- flattenedVisible(flattened)
  println(show(visible == "visible"))
  let ambiguous = Ambiguous{l: 1, r: 2, own: 3}
  println(show(echoAmbiguous(ambiguous) == ambiguous))
  deadline <- conc.withTimeout(1000, hasDeadline())
  println(show(deadline))
  optional <- contextOption(false)
  println(show(optional == None))
  let values = [dyn.Float(1.5), dyn.Time(11), dyn.Time(0), dyn.List([dyn.Str("nested")]), dyn.Map(map.mapFromList([("key", dyn.Int(8))]))]
  println(show(echoDynamic(values) == values))
  println(show(decodeDynamic() == [dyn.Int(-1), dyn.Int(2), dyn.Float(1.5), dyn.Bool(true), dyn.Str("s"), dyn.Bytes("b"), dyn.Time(0), dyn.Null, dyn.Str("{3}")]))
  println(show(namedCallback(r.point, func(p) => p) == r.point))
  negative <- io.tryIO(counterScan(counter, -1))
  println(match negative { Err(e) => show(e); Ok(_) => "wrong" })
 }
}
`)
	out, stderr, status = toolsCommand(t, dir, "", "run", ".")
	const want = "true\n5\ntrue\ntrue\n123\n456\n0\ncopied\n19\ntrue\ntrue\ncontext\n9\nint64:7|string:str|[]uint8:[98 121 116 101 115]|bool:true|<nil>:<nil>\nInt(42)\n6\n7\n3\n[Int(12), Bytes(\"scan\"), Null]\nopaque\ncallback failure\ntrue\ntrue\ntrue\ntrue\ntrue\ntrue\ntrue\ntrue\ngo4t scan: negative count -1\n"
	if out != want || stderr != "" || status != 0 {
		t.Fatalf("run: stdout=%q stderr=%q status=%d", out, stderr, status)
	}
}
