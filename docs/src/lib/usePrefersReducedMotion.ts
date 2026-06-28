import { useEffect, useState } from "react";

// Tracks the user's reduced-motion preference. Returns null while unknown
// (SSR/first paint, before matchMedia runs), then the real boolean after mount —
// so callers don't treat "unknown" as "safe to animate".
export function usePrefersReducedMotion() {
  const [reduced, setReduced] = useState<boolean | null>(null);

  useEffect(() => {
    const query = window.matchMedia("(prefers-reduced-motion: reduce)");
    setReduced(query.matches);
    const onChange = () => setReduced(query.matches);
    query.addEventListener("change", onChange);
    return () => query.removeEventListener("change", onChange);
  }, []);

  return reduced;
}
