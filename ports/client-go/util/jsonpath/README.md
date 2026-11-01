# Native JSONPath

Port of `util/jsonpath` at client-go v0.37.2, revision `72c52dd0cc3b9099b36af11290b0304045dd1ada`. Parser, AST, selectors, range execution, comparisons, and output assembly are go4t. `encoding/` contains only generic byte access and scalar formatting/JSON encoding; it neither parses expressions nor selects input values and imports no client-go code. The upstream Apache license is retained in `LICENSE`.

## Public API and source mapping

All `Result[E,A]` signatures below put the error first. Paths and parsers are immutable; parsing or changing a flag returns a replacement, never mutates an earlier value.

| Upstream export | Native equivalent |
| --- | --- |
| `JSONPath`, `New` | `JSONPath`, `new(name)` |
| `JSONPath.AllowMissingKeys` | `allowMissingKeys(path, allow)` |
| `JSONPath.EnableJSONOutput` | `enableJSONOutput(path, enable)` |
| `JSONPath.Parse` | `jsonPathParse(path, text) Result[String,JSONPath]` |
| `JSONPath.FindResults` | `findResults[A](inspection, path, input) Result[String,List[List[Value[A]]]]` |
| `JSONPath.PrintResults` | `printResults[A](inspection, path, values) Result[String,String]` |
| `JSONPath.Execute` | `execute[A](inspection, path, input) Result[String,String]`; caller writes returned text using its IO writer |
| `Parser`, `NewParser` | `Parser {name,root}`, `newParser(name)` |
| `Parse` | `parse(name,text) Result[String,Parser]` |
| `Parser.Parse` | `parserParse(parser,text) Result[String,Parser]` |
| `ErrSyntax` | `errSyntax()` returns `invalid syntax` |
| `UnquoteExtend` | `unquoteExtend(text) Result[String,String]` |
| `Node` | Exhaustive `Node` sum with all twelve upstream variants |
| `ListNode`, `TextNode`, `FieldNode`, `IdentifierNode` | Same-named variants holding nodes/text/field/name |
| `ArrayNode`, `ParamsEntry` | `ArrayNode(start,end,step)`; `ParamsEntry {value,known,derived}` |
| `FilterNode` | `FilterNode(left,right,operator)` |
| `IntNode`, `FloatNode`, `BoolNode` | Same-named exact scalar variants |
| `WildcardNode`, `RecursiveNode`, `UnionNode` | Same-named variants; union retains branch-major order |
| `NodeType`, all twelve `Node*` constants | Same-named native enum in original ordinal order |
| `Node.Type`, embedded `NodeType.Type` | `nodeType(node)`, `nodeTypeType(kind)` |
| `NodeTypeName`, `NodeType.String` | Caller-owned `Map[NodeType,String]`, initialized by `defaultNodeTypeNames()`; `nodeTypeName(names,kind)` |
| Every node's `String` method | `nodeString(names,node)`, including original six-decimal float diagnostic formatting |

`Parser.root` holds the contents of upstream `Root.Nodes`; wrap in `ListNode` to obtain the root node. Native ADT pattern matching exposes every original node payload without reflection. `upstream-coverage.json` inventories **every upstream exported declaration**, including test-only declarations, original signatures, and every table row. Test-only helper types/functions are represented by native test fixtures, not production APIs.

### Caller-owned node names

The upstream mutable process-global `NodeTypeName` map is replaced by an explicit
persistent `Map[NodeType,String]`. `defaultNodeTypeNames()` supplies all twelve
built-in names. Use `std/map.mapInsert(names,kind,name)` to obtain an updated
registry, or `mapDelete(names,kind)` to remove a name, and pass that value to
`nodeTypeName(names,kind)` and `nodeString(names,node)`. Retain and reuse the registry
for a rendering session; previous values and independently initialized registries
are unchanged. No Go global or implicit default lookup participates in rendering.

Missing entries return the empty string, matching upstream map lookup; they do not
fall back to the built-in name. Explicit empty names behave the same way. Node
payload formatting remains unchanged, including the `": "` separator for scalar
and array nodes. Filter diagnostics obtain both nested list labels from the same
registry: changing `NodeList` to `"Sequence"` renders a filter as
`"NodeFilter: Sequence < Sequence"`. This naming policy does not affect parsing or
JSONPath evaluation.

`testCallerOwnedNodeNames` proves customized type, node and nested filter rendering,
independent registries, deletion and empty-name behavior. `testNodeNameFormatting`
covers all default labels and each variant's customized diagnostic formatting.
Supervisor-released focused validation ran
`go run ./cmd/go4t test ports/client-go/util/jsonpath` through
`serialize-vm-tests --class short` with a 180-second bound: **21 passed, 0 failed**,
including both new cases and the existing parser fixtures. A separate temporary
public consumer ran via `go run ./cmd/go4t run
ports/client-go/util/jsonpath/node-name-consumer` under the same admission and
bound. Its nine assertions passed, covering customized type/node/nested labels,
independent registries and missing mappings; it printed:

```
custom=Sequence; node=Sequence; nested=NodeFilter: Sequence < Sequence
independent=NodeFilter: NodeList < NodeList; missing=NodeFilter:  < 
public node-name consumer: 9 assertions passed
```

The temporary consumer and its generated output were removed after the run.
The historical verification below predates this registry change.

## Typed input inspection

`Value[A] = Dynamic(dyn.Value) | Host(A)` accepts existing native dynamic values or an application-defined immutable record/snapshot. There is no JSON serialization round-trip for field inspection. `dyn.Int` remains signed 64-bit; no selector converts it through Float. `dyn.Bytes` exposes byte elements and uses base64 JSON rendering. `dyn.Time` is inspected as its exact signed timestamp.

```
Inspection[A] {
  inspect: func(A) Shape[A],
  render: func(A,RenderMode) Result[String,String]
}
```

`Shape[A]` distinguishes nil, scalar, sequence, mapping, record, pointer, interface and non-inspectable values. `template.Scalar` distinguishes signed integers, unsigned high/low uint32 limbs, floats, booleans, strings, complex numbers and invalid/non-comparable values. Unsigned limbs must be in `[0,4294967295]`.

Record `Field[A]` carries source name, the first component of its JSON tag (empty when absent/empty), anonymity, and its original typed value. Field policy is native: first exact JSON tag match, last empty-tag anonymous struct recursion, then source-name lookup including breadth-first promoted anonymous fields with ambiguity rejection. Pointer/interface traversal stops at method-bearing interfaces. Sequence/mapping metadata includes its original diagnostic type name; mappings explicitly declare whether a string selector converts to their key type. Adapters expose fields in source declaration order and map entries in their selected iteration order. Native `dyn.Map` uses deterministic key order; upstream map iteration is unspecified.

Inspection and rendering callbacks may access **only immutable data**. An identity-bearing external object must first be snapshotted in IO, then passed here. This package does not claim that opaque arbitrary Go values automatically implement reflection, Stringer, error, tags, or pointer-addressability semantics. A typed adapter must describe them explicitly. Host rendering uses `PlainText`, `CompactJSON`, or `IndentedJSON`; it must implement its own snapshot's codec and error/Stringer/addressability behavior and return an error for unprintable values. Indented host output is relative to level zero, without a trailing newline; the engine adjusts nested indentation. Native composite rendering, spaces between result values, four-space JSON-array output and trailing JSON newline are handled by go4t.

## Semantics and errors

All pinned operators are implemented: recursive descent (including the three-dot field spelling), escaped fields, wildcards including string bytes, index/slice/positive step with negative bounds and derived end, union branch ordering, existence and all six comparison filters, literals, and nested/sibling/empty ranges. Range execution retains outer input after the loop and validates an empty range body against nil, as upstream does. Missing-key policy is aggregate per field step, not an error for every missing member when another member matches. Existence filters ignore missing-field errors; comparison filters honor the selected policy. Text literals still produce one value with an empty input set; numeric/boolean literals preserve input cardinality.

Original stable diagnostics include `unclosed action`, `unterminated array`, `unterminated filter`, `unterminated quoted string`, `invalid multiple recursive descent`, `unclosed array expect ]`, `unrecognized identifier NAME`, `NAME is not found`, `not in range, nothing to end`, `step must be > 0`, `can only compare one element at a time`, `unrecognized filter operator OP`, and original bounds/number/index/type-conversion diagnostics. Native adapter diagnostic type names determine type-bearing messages. No arbitrary reflection panic behavior is promised for malformed foreign adapters or cyclic snapshots; snapshots supplied to recursive traversal must be acyclic, as ordinary native values are.

## Regression inventory and integrated commands

`pinned_test.gt`, `filter_test.gt`, and `native_test.gt` contain all 124 pinned evaluator table rows, all 25 parser table rows (including its malformed row), six exact parser failure rows, and the complete parser AST-string comparison matrix. They preserve the ten evaluator test families: types, struct, JSON, Kubernetes, empty range, nested ranges, partial missing annotations, negative indices, running pods and steps. Additional fixtures cover exact integers above 2^53, native tagged/embedded records, JSON output, byte/unicode/octal escapes, malformed expressions, and the incomplete-template error. The template module carries comparison and explicit reflection fixtures.

Supervisor-authorized focused verification completed through `serialize-vm-tests`: template **3 passed, 0 failed**; JSONPath **19 passed, 0 failed**. The initial template check exposed unqualified `fmap`; imported monadic calls were qualified with `prelude` throughout the owned modules before the passing checks. Exact child commands, run from the worktree root:

```
go run ./cmd/go4t test ports/client-go/third_party/forked/golang/template
go run ./cmd/go4t test ports/client-go/util/jsonpath
```

These are focused native behavioral checks, not source-text or generated-wire assertions. Inspect any compiler/ABI errors before accepting the port; source presence is not verification evidence.

A separate throwaway consumer imported the public JSONPath API (no fixture helpers), queried nested names and an exact integer above 2^53, and printed:

```
alpha beta
9007199254740993
error=unclosed action
```

It ran with `go run ./cmd/go4t run ports/client-go/util/jsonpath/consumer-smoke` through the same serialized runner. The smoke source and generated output directory were removed after success. No repo-wide checks or formatters ran.
