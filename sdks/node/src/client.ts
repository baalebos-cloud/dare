/**
 * Talks to the dare core binary to get a diagnosis for a caught error.
 * Kept thin on purpose: all provider routing/fallback logic lives once in
 * the Go core, not duplicated per language.
 */
import { spawn } from "node:child_process";
import which from "which";

export interface Diagnosis {
  summary: string;
  suggestedFix: string;
  provider: string;
  raw: string;
}

export class CoreNotFoundError extends Error {
  constructor() {
    super(
      "The `dare` core binary was not found on PATH. " +
        "Install it: curl -fsSL https://dare.dev/install.sh | sh"
    );
    this.name = "CoreNotFoundError";
  }
}

async function findCoreBinary(): Promise<string> {
  try {
    return await which("dare");
  } catch {
    throw new CoreNotFoundError();
  }
}

function buildPayload(err: Error): string {
  return JSON.stringify({
    language: "javascript",
    runtime_version: process.version,
    os: process.platform,
    traceback: err.stack ?? `${err.name}: ${err.message}`,
  });
}

export interface DiagnoseOptions {
  timeoutMs?: number;
}

/** Runs the core binary with --json, writes payload to its stdin, and
 * resolves with combined stdout. Uses spawn (not execFile) because piping
 * input to a child's stdin isn't supported by the promisified execFile API.
 */
function runCoreBinary(binary: string, payload: string, timeoutMs: number): Promise<string> {
  return new Promise((resolve, reject) => {
    const child = spawn(binary, ["--json"]);
    let stdout = "";
    let stderr = "";
    let settled = false;

    const timer = setTimeout(() => {
      if (settled) return;
      settled = true;
      child.kill();
      reject(new Error(`dare core timed out after ${timeoutMs}ms`));
    }, timeoutMs);

    child.stdout.on("data", (chunk) => (stdout += chunk));
    child.stderr.on("data", (chunk) => (stderr += chunk));

    child.on("error", (err) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      reject(err);
    });

    child.on("close", (code) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      // Non-zero exit isn't necessarily fatal — the core exits 2 when no
      // provider was reachable but may still have written diagnostic text.
      if (code !== 0 && code !== 2 && !stdout) {
        reject(new Error(`dare core exited unexpectedly (code ${code}): ${stderr}`));
        return;
      }
      resolve(stdout);
    });

    child.stdin.write(payload);
    child.stdin.end();
  });
}

/**
 * Send a caught error's stack trace to the core CLI and return a
 * structured diagnosis. Throws CoreNotFoundError if the binary is
 * missing rather than failing silently.
 */
export async function diagnose(
  err: Error,
  options: DiagnoseOptions = {}
): Promise<Diagnosis> {
  const binary = await findCoreBinary();
  const payload = buildPayload(err);
  const timeout = options.timeoutMs ?? 30_000;

  const stdout = await runCoreBinary(binary, payload, timeout);

  try {
    const data = JSON.parse(stdout);
    return {
      summary: data.summary ?? "",
      suggestedFix: data.suggested_fix ?? "",
      provider: data.provider ?? "unknown",
      raw: stdout,
    };
  } catch {
    // Core printed human-readable text (no --json support yet in this
    // PoC) — surface it directly rather than throwing.
    return { summary: stdout.trim(), suggestedFix: "", provider: "unknown", raw: stdout };
  }
}
