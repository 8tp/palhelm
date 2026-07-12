import type { QueryClient } from "@tanstack/react-query";
import type { ConfigDoc, ConfigSetting } from "./types";

function isSetting(value: unknown): value is ConfigSetting {
  if (typeof value !== "object" || value === null) return false;
  const setting = value as Record<string, unknown>;
  return (
    typeof setting.key === "string" &&
    ["string", "integer", "number", "boolean"].includes(String(setting.type)) &&
    typeof setting.group === "string" &&
    typeof setting.pending === "boolean" &&
    typeof setting.editable === "boolean" &&
    typeof setting.readOnly === "boolean" &&
    setting.editable !== setting.readOnly
  );
}

/** Validate the backend document at the frontend boundary before it enters Query cache. */
export function parseConfigDoc(value: unknown): ConfigDoc {
  if (typeof value !== "object" || value === null) throw new TypeError("Config response is not an object");
  const doc = value as Record<string, unknown>;
  const capabilities = doc.capabilities as Record<string, unknown> | undefined;
  const write = capabilities?.write as Record<string, unknown> | undefined;
  const apply = capabilities?.apply as Record<string, unknown> | undefined;
  if (
    (doc.source !== "compose" && doc.source !== "ini") ||
    typeof doc.service !== "string" ||
    typeof doc.manualCommand !== "string" ||
    typeof write?.available !== "boolean" ||
    typeof apply?.available !== "boolean" ||
    !Array.isArray(doc.settings) ||
    !doc.settings.every(isSetting)
  ) {
    throw new TypeError("Config response does not match the frontend contract");
  }
  return value as ConfigDoc;
}

/** Install only a validated complete ConfigDoc, never the former bare settings array. */
export function setConfigCache(queryClient: Pick<QueryClient, "setQueryData">, doc: ConfigDoc): void {
  queryClient.setQueryData(["config"], parseConfigDoc(doc));
}
