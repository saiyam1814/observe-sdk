import { HoneycombAdapter, HoneycombConfig } from "../../dist/esm/index.js";
import Context from "https://deno.land/std@0.192.0/wasi/snapshot_preview1.ts";
import { load } from "https://deno.land/std/dotenv/mod.ts";

const env = await load();
const apiKey = env["HONEYCOMB_API_KEY"];

const config: HoneycombConfig = {
  apiKey: apiKey,
  dataset: 'deno',
  emitTracesInterval: 1000,
  traceBatchMax: 100,
  host: 'https://api.honeycomb.io',
}
const adapter = new HoneycombAdapter(config);
const opts = {
  spanFilter: {
    minDurationMicroseconds: 100,
  }
};

const bytes = await Deno.readFile("../../test-data/test.c.instr.wasm");
const traceContext = await adapter.start(bytes, opts);
const module = new WebAssembly.Module(bytes);

const runtime = new Context({
  stdin: Deno.stdin.rid,
  stdout: Deno.stdout.rid,
});
const instance = new WebAssembly.Instance(
  module,
  {
    "wasi_snapshot_preview1": runtime.exports,
    ...traceContext.getImportObject(),
  },
);
runtime.start(instance);

traceContext.stop();

setTimeout(() => { }, 3000);
