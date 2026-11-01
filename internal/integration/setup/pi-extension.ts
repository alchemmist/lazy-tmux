// Managed by lazy-tmux: pi-extension-v1
import { realpathSync } from "node:fs";
import { execFileSync } from "node:child_process";
import type { ExtensionAPI, ExtensionContext } from "@earendil-works/pi-coding-agent";

export default function (pi: ExtensionAPI) {
  if (!process.env.TMUX || !process.env.TMUX_PANE) return;
  const tmux = (args: string[]) => execFileSync("tmux", args, {
    timeout: 2000, encoding: "utf8", stdio: ["ignore", "pipe", "ignore"],
  });
  const startTime = (pid: number) => execFileSync("ps", ["-p", String(pid), "-o", "lstart="], {
    timeout: 2000, encoding: "utf8", env: { ...process.env, LC_ALL: "C", TZ: "UTC" },
  }).trim().replace(/\s+/g, " ");
  let startedAt = "";
  try { startedAt = startTime(process.pid); }
  catch { /* Without ownership proof lazy-tmux must leave the pane unbound. */ }
  const owns = (binding: any) => binding.pid === process.pid && binding.started_at === startedAt;
  const foreignOwnerAlive = (binding: any) => {
    if (!Number.isInteger(binding.pid) || binding.pid <= 0 || !binding.started_at) return true;
    try { return startTime(binding.pid) === binding.started_at; }
    catch {
      // A failed ps is not proof of exit (it may be unavailable).
      try { process.kill(binding.pid, 0); return true; }
      catch (error: any) { return error.code !== "ESRCH"; }
    }
  };
  const quote = (value: string) => "'" + value.replace(/'/g, "'\\''") + "'";
  const literal = (value: string) => value.replace(/#/g, "##").replace(/,/g, "#,").replace(/}/g, "#}");
  const readBinding = () => {
    try { return tmux(["show-option", "-p", "-v", "-t", process.env.TMUX_PANE!, "@lazy_tmux_pi_binding"]).trim(); }
    catch { return ""; }
  };
  // Compare and mutate in tmux's command queue, not in separate read/set calls:
  // another client may have acquired the pane after our ownership check.
  const replaceBinding = (previous: string, next?: string, ack?: string) => {
    const pane = process.env.TMUX_PANE!;
    const command = next === undefined
      ? ["set-option", "-pu", "-t", pane, "@lazy_tmux_pi_binding"]
      : ["set-option", "-p", "-t", pane, "@lazy_tmux_pi_binding", next];
    let commands = command.map(quote).join(" ");
    if (ack) commands += " ; " + ["set-option", "-p", "-t", pane, "@lazy_tmux_restore_ack", ack].map(quote).join(" ");
    tmux(["if-shell", "-F", "-t", pane,
      "#{==:#{@lazy_tmux_pi_binding}," + literal(previous) + "}", commands]);
  };
  const sameDirectory = (left: string, right: string) => {
    if (!left || !right) return false;
    if (left === right) return true;
    try { return realpathSync(left) === realpathSync(right); } catch { return false; }
  };
  const restoreAck = (file: string | undefined, ctx: ExtensionContext) => {
    const token = process.env.LAZY_TMUX_RESTORE_TOKEN;
    if (!token || !file) return undefined;
    try {
      const encoded = tmux(["show-option", "-p", "-v", "-t", process.env.TMUX_PANE!, "@lazy_tmux_restore_pending"]).trim();
      const intent = JSON.parse(Buffer.from(encoded, "base64").toString("utf8"));
      const pane = intent.pane;
      const meta = pane.meta ?? {};
      const id = ctx.sessionManager.getSessionId();
      if (intent.token !== token || (!meta["pi.session_id"] && !pane.agent?.id)) return undefined;
      if ((meta["pi.session_id"] && meta["pi.session_id"] !== id) ||
          (meta["pi.session_file"] && meta["pi.session_file"] !== file) ||
          !sameDirectory(pane.current_path, ctx.cwd)) return undefined;
      if (pane.agent && (pane.agent.kind !== "pi" || pane.agent.id !== id ||
          pane.agent.argv?.[3] !== file || !sameDirectory(pane.agent.cwd, ctx.cwd))) return undefined;
      return token;
    } catch { return undefined; }
  };
  let published = false;
  let running = false;
  let compacting = false;
  let settledState = "idle";
  let compactionSignal: AbortSignal | undefined;
  let onCompactionAbort: (() => void) | undefined;
  const clearCompaction = () => {
    if (compactionSignal && onCompactionAbort) {
      compactionSignal.removeEventListener("abort", onCompactionAbort);
    }
    compactionSignal = undefined;
    onCompactionAbort = undefined;
    compacting = false;
  };
  const prompts: string[] = [];
  const state = () => {
    if (prompts.length) {
      return ["confirm", "select"].includes(prompts[prompts.length - 1])
        ? "awaiting_decision" : "awaiting_input";
    }
    return running || compacting ? "working" : settledState;
  };
  const publish = (_event: unknown, ctx: ExtensionContext) => {
    if (!process.env.TMUX || !process.env.TMUX_PANE || !startedAt) return;
    // RPC/print clients inherit TMUX_PANE from interactive tools. They must
    // never publish an empty binding, acknowledge a restore, or clear an owner.
    if (!ctx.hasUI || (ctx.mode !== undefined && ctx.mode !== "tui")) return;
    const file = ctx.sessionManager.getSessionFile();
    try {
      const previous = readBinding();
      if (previous) {
        const binding = JSON.parse(previous);
        if (!owns(binding) && foreignOwnerAlive(binding)) return;
      }
      replaceBinding(previous, JSON.stringify({ version: 1, pid: process.pid, started_at: startedAt, file: file ?? "",
        session_id: file ? ctx.sessionManager.getSessionId() : "", state: file ? state() : "" }),
        restoreAck(file, ctx));
      published = true;
    } catch { /* Never disrupt a conversation when tmux is unavailable. */ }
  };

  const resetSession = (_event: unknown, ctx: ExtensionContext) => {
    running = !ctx.isIdle();
    clearCompaction();
    prompts.length = 0;
    settledState = "idle";
    publish(_event, ctx);
  };
  pi.on("session_start", resetSession);
  // Unknown names are inert: upstream emits switch/fork separately, newer Pi
  // emits session_start with a reason. Optional prompt events are not guessed
  // from terminal output on clients that do not expose them.
  const onLifecycle = pi.on.bind(pi) as (
    event: string, handler: (event: any, ctx: ExtensionContext) => void
  ) => unknown;
  onLifecycle("session_switch", resetSession);
  onLifecycle("session_fork", resetSession);
  const start = (_event: unknown, ctx: ExtensionContext) => {
    running = true;
    settledState = "awaiting_input";
    publish(_event, ctx);
  };
  pi.on("before_agent_start", start);
  pi.on("agent_start", start);
  pi.on("agent_end", (event, ctx) => {
    running = false;
    clearCompaction();
    prompts.length = 0;
    const last = [...event.messages].reverse().find(message => message.role === "assistant");
    settledState = last?.stopReason === "error" ? "error" : "awaiting_input";
    publish(event, ctx);
  });
  onLifecycle("agent_settled", (event, ctx) => {
    running = false;
    clearCompaction();
    prompts.length = 0;
    if (event.aborted) settledState = "awaiting_input";
    publish(event, ctx);
  });
  pi.on("session_before_compact", (event, ctx) => {
    clearCompaction();
    compacting = true;
    compactionSignal = event.signal;
    onCompactionAbort = () => {
      clearCompaction();
      running = !ctx.isIdle();
      settledState = "awaiting_input";
      publish(event, ctx);
    };
    if (compactionSignal?.aborted) { onCompactionAbort(); return; }
    compactionSignal?.addEventListener("abort", onCompactionAbort, { once: true });
    publish(event, ctx);
  });
  pi.on("session_compact", (event, ctx) => {
    clearCompaction();
    running = !ctx.isIdle();
    settledState = "awaiting_input";
    publish(event, ctx);
  });
  onLifecycle("session_compact_failed", (event, ctx) => {
    clearCompaction();
    running = !ctx.isIdle();
    settledState = event.aborted ? "awaiting_input" : "error";
    publish(event, ctx);
  });
  onLifecycle("ui_prompt_start", (event, ctx) => {
    prompts.push(event.kind);
    publish(event, ctx);
  });
  onLifecycle("ui_prompt_end", (event, ctx) => {
    const index = prompts.lastIndexOf(event.kind);
    if (index >= 0) prompts.splice(index, 1);
    publish(event, ctx);
  });
  pi.on("session_shutdown", () => {
    if (!published || !process.env.TMUX || !process.env.TMUX_PANE || !startedAt) return;
    try {
      const previous = readBinding();
      if (previous && owns(JSON.parse(previous))) replaceBinding(previous);
    } catch { /* The pane may already be gone. */ }
  });
}
