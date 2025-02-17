import { Application, Router } from 'https://deno.land/x/oak@v12.6.0/mod.ts'
import Context from 'https://deno.land/std@0.192.0/wasi/snapshot_preview1.ts'
import { tmpdir } from 'node:os'
import { DatadogAdapter } from '../../../js/packages/observe-sdk-datadog/dist/esm/index.js'

const config = {
    agentHost: new URL('http://ddagent:8126'),
    serviceName: 'iota-deno',
    defaultTags: new Map(),
    traceType: 'node',
    emitTracesInterval: 1000, // milliseconds
    traceBatchMax: 100
}
const adapter = new DatadogAdapter(config)

const app = new Application()
const router = new Router()

router.get('/', (ctx) => {
    ctx.response.body = 'Hi'
})

router.post('/upload', async (ctx) => {
    try {
        const req = ctx.request
        const qs = new URLSearchParams(req.url.search)
        const body = req.body()
        if (body.type === 'form-data') {
            const value = body.value
            for await (const [name, v] of value.stream({ maxSize: 1_000_000_000 })) {
                if (name === 'wasm') {
                    const filename = `${tmpdir()}/${qs.get('name')}.wasm`
                    // truncate the carriage return / line feed that gets appended to the form data
                    await Deno.writeFile(filename, v.content.slice(0, v.content.length - 2))
                    console.log(`Successfully uploaded ${filename}`)
                }
            }
        }
        ctx.response.status = 200
        ctx.response.body = ''
    } catch (e) {
        console.error(e)
        ctx.response.status = 500
    }
})

router.post('/run', async (ctx) => {
    try {
        const stdinPath = `${tmpdir()}/stdin_${Math.ceil(Math.random() * 10000)}.txt`
        const stdoutPath = `${tmpdir()}/stdout_${Math.ceil(Math.random() * 10000)}.txt`
        const stdin = await Deno.create(stdinPath)
        const stdout = await Deno.create(stdoutPath)

        const req = ctx.request
        const qs = new URLSearchParams(req.url.search)
        const bytes = await Deno.readFile(`${tmpdir()}/${qs.get('name')}.wasm`)
        const traceContext = await adapter.start(bytes)
        const module = new WebAssembly.Module(bytes)

        const bodyStream = req.body({ type: "stream" })
        await bodyStream.value.pipeTo(stdin.writable, { preventClose: true });
        await stdin.seek(0, Deno.SeekMode.Start);

        const runtime = new Context({
            args: [qs.get('name')!],
            env: {},
            stdin: stdin.rid,
            stdout: stdout.rid,
        })
        const instance = new WebAssembly.Instance(
            module,
            {
                'wasi_snapshot_preview1': runtime.exports,
                ...traceContext.getImportObject(),
            },
        )
        runtime.start(instance)

        traceContext.setMetadata({
            http_status_code: '200',
            http_url: `${req.proto}://${req.headers['host']}${req.originalUrl}`,
        });
        traceContext.stop()
        ctx.response.status = 200

        await stdout.seek(0, Deno.SeekMode.Start);
        ctx.response.body = stdout.readable
        Deno.remove(stdoutPath)
        Deno.remove(stdinPath)
    } catch (e) {
        console.error(e)
        ctx.response.status = 500
    }
})

app.use(router.routes())
app.use(router.allowedMethods())

const port = 3000
console.log(`Listening on port: ${port}`)
await app.listen({ port })
