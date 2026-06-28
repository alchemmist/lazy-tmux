import { useEffect, useRef } from "react";
import { usePrefersReducedMotion } from "../lib/usePrefersReducedMotion";

interface DemoVideoProps {
  src: string;
}

// A looping, muted demo video. It plays on its own only when the user hasn't
// asked to reduce motion; otherwise it stays paused on its poster frame and the
// controls let them start it manually.
export function DemoVideo({ src }: DemoVideoProps) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const reducedMotion = usePrefersReducedMotion();

  useEffect(() => {
    if (!reducedMotion) {
      videoRef.current?.play().catch(() => {
        /* autoplay blocked — controls remain available */
      });
    }
  }, [reducedMotion]);

  return (
    <video
      ref={videoRef}
      controls
      muted
      loop
      playsInline
      preload="metadata"
      width="100%"
    >
      <source src={src} type="video/mp4" />
      Your browser does not support the video tag.
    </video>
  );
}
