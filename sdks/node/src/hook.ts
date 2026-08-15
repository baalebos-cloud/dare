/**
 * Optional auto-hook: listens for uncaughtException / unhandledRejection,
 * diagnoses it, prints the diagnosis, then lets the process exit normally
 * (Node exits after an uncaughtException handler returns unless you stop
 * it — we deliberately don't stop it, so behavior stays predictable).
 */
import { diagnose, CoreNotFoundError, type Diagnosis } from "./client.js";

let installed = false;

async function report(err: Error): Promise<void> {
  try {
    const result: Diagnosis = await diagnose(err);
    process.stderr.write(`\n✖ dare diagnosis (via ${result.provider}):\n`);
    process.stderr.write(`  ${result.summary}\n`);
    if (result.suggestedFix) {
      process.stderr.write(`\n  Suggested fix:\n  ${result.suggestedFix}\n`);
    }
  } catch (e) {
    if (e instanceof CoreNotFoundError) {
      process.stderr.write(`\n(dare: ${e.message})\n`);
    }
    // Never let the diagnosis path itself crash harder than the original error.
  }
}

function onUncaughtException(err: Error): void {
  report(err).finally(() => {
    // Print the original error too so nothing is lost, then exit — do NOT
    // re-throw here: this callback runs asynchronously (after report's
    // promise settles), so a throw would fire a *new* uncaughtException
    // event while this same listener is still installed, risking an
    // infinite loop instead of a clean exit.
    console.error(err);
    process.exit(1);
  });
}

function onUnhandledRejection(reason: unknown): void {
  const err = reason instanceof Error ? reason : new Error(String(reason));
  void report(err);
}

/** Install the global hook. Idempotent. */
export function install(): void {
  if (installed) return;
  process.on("uncaughtException", onUncaughtException);
  process.on("unhandledRejection", onUnhandledRejection);
  installed = true;
}

export function uninstall(): void {
  process.off("uncaughtException", onUncaughtException);
  process.off("unhandledRejection", onUnhandledRejection);
  installed = false;
}
