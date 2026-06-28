import { useCallback, useState } from "react";

interface InlineCodeProps {
  /** The literal text shown and copied on click. */
  children: string;
}

// Inline code chip that copies its own text to the clipboard when clicked
// (or activated by keyboard), with a brief "copied" hint.
export function InlineCode({ children }: InlineCodeProps) {
  const [copied, setCopied] = useState(false);

  const copy = useCallback(() => {
    if (typeof navigator === "undefined" || !navigator.clipboard) {
      return;
    }
    navigator.clipboard
      .writeText(children)
      .then(() => {
        setCopied(true);
        window.setTimeout(() => setCopied(false), 1200);
      })
      .catch(() => {
        /* clipboard denied — nothing to do */
      });
  }, [children]);

  return (
    <code
      className={`inline-code${copied ? " is-copied" : ""}`}
      role="button"
      tabIndex={0}
      title="Click to copy"
      onClick={copy}
      onKeyDown={(event) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          copy();
        }
      }}
    >
      {children}
      {copied && <span className="inline-code__hint">copied</span>}
    </code>
  );
}
