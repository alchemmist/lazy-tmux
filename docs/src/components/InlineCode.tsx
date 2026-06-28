import { Tooltip } from "@gravity-ui/uikit";
import { useCopy } from "../lib/useCopy";

interface InlineCodeProps {
  /** The literal text shown and copied on click. */
  children: string;
}

// Inline code chip that copies its own text to the clipboard when clicked (or
// activated by keyboard). A Gravity Tooltip hints the affordance; a Gravity
// toast confirms the copy.
export function InlineCode({ children }: InlineCodeProps) {
  const copy = useCopy();

  return (
    <Tooltip content="Click to copy">
      <code
        className="inline-code"
        role="button"
        tabIndex={0}
        onClick={() => copy(children)}
        onKeyDown={(event) => {
          if (event.key === "Enter" || event.key === " ") {
            event.preventDefault();
            copy(children);
          }
        }}
      >
        {children}
      </code>
    </Tooltip>
  );
}
