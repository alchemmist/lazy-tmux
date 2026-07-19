import { useEffect, useState } from "react";

const RELEASES = "https://github.com/alchemmist/lazy-tmux/releases";
const LATEST = "https://api.github.com/repos/alchemmist/lazy-tmux/releases/latest";

export function Version() {
  const [tag, setTag] = useState<string | null>(null);

  useEffect(() => {
    let alive = true;

    fetch(LATEST)
      .then((response) => (response.ok ? response.json() : null))
      .then((data) => {
        if (alive && data && typeof data.tag_name === "string") {
          setTag(data.tag_name);
        }
      })
      .catch(() => {});

    return () => {
      alive = false;
    };
  }, []);

  if (!tag) {
    return null;
  }

  return (
    <a
      className="version"
      href={`${RELEASES}/tag/${tag}`}
      target="_blank"
      rel="noopener noreferrer"
    >
      {tag}
    </a>
  );
}
