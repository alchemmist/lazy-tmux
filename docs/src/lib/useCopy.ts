import { useCallback } from "react";
import { useToaster } from "@gravity-ui/uikit";

// Copies text to the clipboard and pops a Gravity success toast. Returns a
// stable callback; safe to call from click/keyboard handlers.
export function useCopy() {
  const toaster = useToaster();

  return useCallback(
    (text: string) => {
      if (typeof navigator === "undefined" || !navigator.clipboard) {
        return;
      }
      navigator.clipboard
        .writeText(text)
        .then(() => {
          toaster.add({
            name: "copy",
            title: "Copied to clipboard",
            theme: "success",
            autoHiding: 1500,
          });
        })
        .catch(() => {
          /* clipboard denied — nothing to do */
        });
    },
    [toaster],
  );
}
