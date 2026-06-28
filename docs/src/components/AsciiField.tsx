import { useEffect, useRef } from "react";

// Density ramp: a 0..1 field value picks a glyph from sparse to dense.
const RAMP = " .·:-=+*#%@";
const CELL_W = 9;
const CELL_H = 15;
const TARGET_FPS = 30;
const WARM = "rgba(255, 210, 90,";
const COOL = "rgba(232, 232, 232,";

// A generative ASCII background: several drifting sine waves interfere into an
// organic, breathing field of monospace glyphs, with a ripple that follows the
// pointer. Pure canvas, SSR-safe (all work happens in the effect), respects
// prefers-reduced-motion, fixed across the whole viewport.
export function AsciiField({ mode = "home" }: { mode?: "home" | "docs" }) {
  const canvasRef = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    const canvasEl = canvasRef.current;
    const context = canvasEl?.getContext("2d");
    if (!canvasEl || !context) {
      return;
    }
    // Non-null locals so the values stay narrowed inside nested closures.
    const canvas = canvasEl;
    const ctx = context;

    const reduceMotion = window.matchMedia(
      "(prefers-reduced-motion: reduce)",
    ).matches;

    let cols = 0;
    let rows = 0;
    const pointer = { x: -1, y: -1, strength: 0 };

    function resize() {
      const rect = canvas.getBoundingClientRect();
      const dpr = Math.min(window.devicePixelRatio || 1, 2);
      canvas.width = Math.max(1, Math.floor(rect.width * dpr));
      canvas.height = Math.max(1, Math.floor(rect.height * dpr));
      // +1 so the field reaches the very right/bottom edge (no hard cutoff).
      cols = Math.floor(rect.width / CELL_W) + 1;
      rows = Math.floor(rect.height / CELL_H) + 1;
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      ctx.font = `${CELL_H - 2}px "JetBrains Mono", monospace`;
      ctx.textBaseline = "top";
    }

    function draw(t: number) {
      ctx.clearRect(0, 0, canvas.width, canvas.height);
      for (let j = 0; j < rows; j++) {
        for (let i = 0; i < cols; i++) {
          const x = i * 0.18;
          const y = j * 0.28;
          let v =
            Math.sin(x + t * 0.6) +
            Math.sin(y * 0.8 - t * 0.5) +
            Math.sin((x + y) * 0.5 + t * 0.4) +
            Math.sin(Math.hypot(x - cols * 0.09, y - rows * 0.14) - t * 0.8);
          v /= 4;
          if (pointer.x >= 0 && pointer.strength > 0.01) {
            const d = Math.hypot(i - pointer.x, (j - pointer.y) * 1.4);
            v +=
              Math.cos(d * 0.45 - t * 3) * Math.exp(-d * 0.08) * pointer.strength;
          }
          v = Math.max(0, Math.min(1, (v + 1) / 2));
          const ch = RAMP[Math.floor(v * (RAMP.length - 1))];
          if (ch === " ") {
            continue;
          }
          const alpha = 0.06 + v * v * 0.7;
          ctx.fillStyle = `${v > 0.86 ? WARM : COOL} ${alpha})`;
          ctx.fillText(ch, i * CELL_W, j * CELL_H);
        }
      }
    }

    resize();
    const resizeObserver = new ResizeObserver(() => {
      resize();
      if (reduceMotion) {
        draw(0);
      }
    });
    resizeObserver.observe(canvas);

    if (reduceMotion) {
      draw(0);
      return () => resizeObserver.disconnect();
    }

    let raf = 0;
    let last = 0;
    let running = true;
    const frameInterval = 1000 / TARGET_FPS;
    function loop(now: number) {
      if (!running) {
        return;
      }
      raf = requestAnimationFrame(loop);
      if (now - last < frameInterval) {
        return;
      }
      last = now;
      pointer.strength *= 0.96; // ripple decays after the pointer stops moving
      draw(now / 1000);
    }
    raf = requestAnimationFrame(loop);

    function onPointerMove(event: PointerEvent) {
      const rect = canvas.getBoundingClientRect();
      pointer.x = (event.clientX - rect.left) / CELL_W;
      pointer.y = (event.clientY - rect.top) / CELL_H;
      pointer.strength = 1.2;
    }
    function onPointerLeave() {
      pointer.x = -1;
      pointer.y = -1;
    }
    window.addEventListener("pointermove", onPointerMove);
    document.addEventListener("pointerleave", onPointerLeave);

    function onVisibility() {
      if (document.hidden) {
        running = false;
        cancelAnimationFrame(raf);
      } else if (!running) {
        running = true;
        last = 0;
        raf = requestAnimationFrame(loop);
      }
    }
    document.addEventListener("visibilitychange", onVisibility);

    return () => {
      running = false;
      cancelAnimationFrame(raf);
      resizeObserver.disconnect();
      window.removeEventListener("pointermove", onPointerMove);
      document.removeEventListener("pointerleave", onPointerLeave);
      document.removeEventListener("visibilitychange", onVisibility);
    };
  }, []);

  return (
    <canvas
      ref={canvasRef}
      className={`ascii-field ascii-field--${mode}`}
      aria-hidden="true"
    />
  );
}
