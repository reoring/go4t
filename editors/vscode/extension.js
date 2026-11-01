'use strict';

const vscode = require('vscode');
const { spawn } = require('node:child_process');

let activeClient;

function range(value) {
  return new vscode.Range(value.start.line, value.start.character, value.end.line, value.end.character);
}

class Client {
  constructor(output, diagnostics) {
    this.output = output;
    this.diagnostics = diagnostics;
    this.pending = new Map();
    this.nextID = 0;
    this.buffer = Buffer.alloc(0);
    this.stopped = false;
    this.process = spawn(vscode.workspace.getConfiguration('gofx').get('executablePath', 'gofx'), ['lsp'], {
      stdio: ['pipe', 'pipe', 'pipe']
    });
    this.process.stderr.on('data', data => output.append(data.toString()));
    this.process.stdout.on('data', data => this.receive(data));
    this.process.on('error', error => this.fail(error));
    this.process.stdin.on('error', error => this.fail(error));
    this.process.on('exit', (code, signal) => this.fail(new Error(`gofx lsp exited (${signal || code})`)));
  }

  send(message) {
    if (this.stopped) throw new Error('gofx language server is stopped');
    const body = Buffer.from(JSON.stringify({ jsonrpc: '2.0', ...message }));
    this.process.stdin.write(`Content-Length: ${body.length}\r\n\r\n`);
    this.process.stdin.write(body);
  }

  request(method, params) {
    return new Promise((resolve, reject) => {
      const id = ++this.nextID;
      this.pending.set(id, { resolve, reject });
      try { this.send({ id, method, params }); }
      catch (error) { this.pending.delete(id); reject(error); }
    });
  }

  notify(method, params) {
    if (!this.stopped) this.send({ method, params });
  }

  receive(data) {
    this.buffer = Buffer.concat([this.buffer, data]);
    for (;;) {
      const boundary = this.buffer.indexOf('\r\n\r\n');
      if (boundary === -1) return;
      const match = /(?:^|\r\n)Content-Length:\s*(\d+)/i.exec(this.buffer.subarray(0, boundary).toString());
      if (!match) { this.fail(new Error('Invalid LSP Content-Length')); this.process.kill(); return; }
      const length = Number(match[1]);
      if (this.buffer.length < boundary + 4 + length) return;
      let message;
      try { message = JSON.parse(this.buffer.subarray(boundary + 4, boundary + 4 + length).toString()); }
      catch (error) { this.fail(error); this.process.kill(); return; }
      this.buffer = this.buffer.subarray(boundary + 4 + length);
      if (Object.hasOwn(message, 'id')) {
        const pending = this.pending.get(message.id);
        if (!pending) continue;
        this.pending.delete(message.id);
        if (message.error) pending.reject(new Error(message.error.message));
        else pending.resolve(message.result);
      } else if (message.method === 'textDocument/publishDiagnostics') {
        const params = message.params;
        const open = vscode.workspace.textDocuments.find(doc => doc.uri.toString() === params.uri);
        if (open && params.version !== undefined && open.version !== params.version) continue;
        this.diagnostics.set(vscode.Uri.parse(params.uri), params.diagnostics.map(item => {
          const diagnostic = new vscode.Diagnostic(range(item.range), item.message,
            item.severity === 2 ? vscode.DiagnosticSeverity.Warning : vscode.DiagnosticSeverity.Error);
          diagnostic.source = item.source;
          return diagnostic;
        }));
      }
    }
  }

  fail(error) {
    if (this.stopped) return;
    this.stopped = true;
    this.output.appendLine(error.message);
    for (const pending of this.pending.values()) pending.reject(error);
    this.pending.clear();
    this.diagnostics.clear();
  }

  async stop() {
    if (this.stopped) return;
    // Deactivation must not keep VS Code waiting if the child has stopped responding.
    const timer = setTimeout(() => this.process.kill(), 2000);
    try {
      await this.request('shutdown', null);
      this.notify('exit');
      this.process.stdin.end();
    } catch (error) { this.output.appendLine(error.message); }
    finally { clearTimeout(timer); if (!this.stopped) this.process.kill(); }
  }
}

async function activate(context) {
  const output = vscode.window.createOutputChannel('gofx');
  const diagnostics = vscode.languages.createDiagnosticCollection('gofx');
  context.subscriptions.push(output, diagnostics);
  const client = new Client(output, diagnostics);
  activeClient = client;
  try {
    await client.request('initialize', { processId: process.pid, capabilities: {}, rootUri: null });
    client.notify('initialized', {});
  } catch (error) {
    vscode.window.showErrorMessage(`Cannot start gofx: ${error.message}`);
    return;
  }
  const isSource = doc => doc.languageId === 'gofx' && doc.uri.scheme === 'file';
  const documentID = doc => ({ uri: doc.uri.toString() });
  const open = doc => {
    if (isSource(doc)) client.notify('textDocument/didOpen', {
      textDocument: { ...documentID(doc), languageId: 'gofx', version: doc.version, text: doc.getText() }
    });
  };
  context.subscriptions.push(
    vscode.workspace.onDidOpenTextDocument(open),
    vscode.workspace.onDidChangeTextDocument(event => {
      if (isSource(event.document)) client.notify('textDocument/didChange', {
        textDocument: { ...documentID(event.document), version: event.document.version },
        contentChanges: [{ text: event.document.getText() }]
      });
    }),
    vscode.workspace.onDidCloseTextDocument(doc => {
      if (isSource(doc)) client.notify('textDocument/didClose', { textDocument: documentID(doc) });
    }),
    vscode.workspace.registerTextDocumentContentProvider('gofx-prelude', {
      provideTextDocumentContent: () => client.request('gofx/prelude', {})
    })
  );
  vscode.workspace.textDocuments.forEach(open);
  const selector = { language: 'gofx', scheme: 'file' };
  const at = (doc, position) => ({ textDocument: documentID(doc), position });
  context.subscriptions.push(
    vscode.languages.registerHoverProvider(selector, {
      async provideHover(doc, position) {
        const result = await client.request('textDocument/hover', at(doc, position));
        if (!result) return undefined;
        const text = new vscode.MarkdownString();
        text.appendText(result.contents.value);
        return new vscode.Hover(text, range(result.range));
      }
    }),
    vscode.languages.registerDefinitionProvider(selector, {
      async provideDefinition(doc, position) {
        const result = await client.request('textDocument/definition', at(doc, position));
        return result ? new vscode.Location(vscode.Uri.parse(result.uri), range(result.range)) : undefined;
      }
    }),
    vscode.languages.registerDocumentSymbolProvider(selector, {
      async provideDocumentSymbols(doc) {
        const symbols = await client.request('textDocument/documentSymbol', { textDocument: documentID(doc) });
        const convert = item => {
          // LSP SymbolKind is one-based; vscode.SymbolKind is zero-based.
          const symbol = new vscode.DocumentSymbol(item.name, item.detail || '', item.kind - 1,
            range(item.range), range(item.selectionRange));
          symbol.children = (item.children || []).map(convert);
          return symbol;
        };
        return symbols.map(convert);
      }
    })
  );
}

async function deactivate() {
  if (activeClient) await activeClient.stop();
  activeClient = undefined;
}

module.exports = { activate, deactivate };
