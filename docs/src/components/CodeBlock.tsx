import { ClipboardButton, useToaster } from "@gravity-ui/uikit";

interface CodeBlockProps {
  /** Raw code to render and copy. Whitespace is preserved verbatim. */
  children: string;
}

// A block of code on a surface that stands clearly above the page background,
// with a Gravity UI copy button (animated check) in the top-right corner.
export function CodeBlock({ children }: CodeBlockProps) {
  const code = children.replace(/\n$/, "");
  const toaster = useToaster();

  return (
    <div className="code-block">
      <div className="code-block__bar">
        <ClipboardButton
          text={code}
          size="s"
          view="flat"
          className="code-block__copy"
          onCopy={() =>
            toaster.add({
              name: "copy-block",
              title: "Copied to clipboard",
              theme: "success",
              autoHiding: 1500,
            })
          }
        />
      </div>
      <pre>
        <code>{code}</code>
      </pre>
    </div>
  );
}
