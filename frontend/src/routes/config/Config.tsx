import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import { setConfigCache } from "../../api/configContract";
import { ApiRequestError, type ConfigSetting } from "../../api/types";
import { useIsAdmin } from "../../app/AuthProvider";
import { Card, CardBody, CardHead } from "../../components/Card";
import { Banner } from "../../components/Banner";
import { Tabs } from "../../components/Tabs";
import { Pill } from "../../components/Pill";
import { CodeWell } from "../../components/CodeWell";
import { useToast } from "../../components/Toast";
import "./Config.css";

const TABS = [
  { key: "editor", label: "Settings editor" },
  { key: "raw", label: "Raw ini (read-only)" },
];

// Option labels for select-typed settings (values are what the game expects in the ini).
const SELECT_OPTIONS: Record<string, { value: string; label: string }[]> = {
  DIFFICULTY: [
    { value: "None", label: "None" },
    { value: "Casual", label: "Casual" },
    { value: "Normal", label: "Normal" },
    { value: "Hard", label: "Hard" },
  ],
  DEATH_PENALTY: [
    { value: "None", label: "None" },
    { value: "Item", label: "Item" },
    { value: "ItemAndEquipment", label: "All items drop" },
    { value: "All", label: "All items and Pals drop" },
  ],
};

export default function ConfigRoute() {
  const [tab, setTab] = useState("editor");

  return (
    <main className="content">
      <div className="page-head">
        <h1>Configuration</h1>
        <span className="sub">PalWorldSettings.ini · via compose environment</span>
      </div>

      <Banner tone="info">
        This server generates PalWorldSettings.ini from docker-compose environment variables on every boot. Palhelm
        edits the compose file — changes apply after a container restart.
      </Banner>

      <Tabs items={TABS} active={tab} onChange={setTab} />

      {tab === "editor" ? <EditorTab /> : <RawIniTab />}
    </main>
  );
}

// ---------------- settings editor ----------------

function EditorTab() {
  const isAdmin = useIsAdmin();
  const queryClient = useQueryClient();
  const toast = useToast();

  const configQuery = useQuery({ queryKey: ["config"], queryFn: () => api.config.get() });
  const [edits, setEdits] = useState<Record<string, string>>({});

  const settings = useMemo(() => configQuery.data?.settings ?? [], [configQuery.data]);
  const byKey = useMemo(() => new Map(settings.map((s) => [s.key, s])), [settings]);

  const dirtyKeys = Object.keys(edits).filter((k) => {
    const s = byKey.get(k);
    return s !== undefined && edits[k] !== String(s.value);
  });
  const dirty = dirtyKeys.length > 0;
  const serverPendingCount = settings.filter((s) => s.pending).length;

  const writeMutation = useMutation({
    mutationFn: () => {
      const changes: Record<string, string> = {};
      for (const k of dirtyKeys) changes[k] = edits[k]!;
      const version = configQuery.data?.version;
      if (!version) throw new Error("The compose version is unavailable; reload configuration.");
      return api.config.put(version, changes);
    },
    onSuccess: (updated) => {
      setConfigCache(queryClient, updated);
      setEdits({});
      toast.push("Written to the compose file — restart the server to apply.", "ok");
    },
    onError: (err) => {
      if (err instanceof ApiRequestError && err.code === "config_conflict") {
        queryClient.invalidateQueries({ queryKey: ["config"] });
        setEdits({});
        toast.push("The compose file changed elsewhere. Configuration was reloaded; review your edit and try again.", "danger");
      } else {
        toast.push("Couldn't write the compose file.", "danger");
      }
    },
  });

  function value(s: ConfigSetting): string {
    return edits[s.key] ?? String(s.value);
  }
  function setValue(key: string, v: string) {
    setEdits((e) => ({ ...e, [key]: v }));
  }
  function isDirty(s: ConfigSetting): boolean {
    return edits[s.key] !== undefined && edits[s.key] !== String(s.value);
  }

  if (configQuery.isError) {
    return (
      <Card>
        <CardBody>
          <Banner tone="warn">Couldn't load the configuration. Check the compose file mount.</Banner>
        </CardBody>
      </Card>
    );
  }
  if (configQuery.isLoading) {
    return (
      <Card>
        <CardBody>
          <span className="skel skel-text" style={{ width: "100%", height: 120 }} />
        </CardBody>
      </Card>
    );
  }

  const groups: string[] = [];
  for (const s of settings) if (!groups.includes(s.group)) groups.push(s.group);

  const writeCapability = configQuery.data!.capabilities.write;

  return (
    <>
      {!writeCapability.available && (
        <Banner tone="warn">
          Configuration is read-only: {writeCapability.reason ?? "the compose deployment does not support safe atomic writes"}.
        </Banner>
      )}
      <div className="grid cols-2">
        {groups.map((group) => (
          <GroupCard
            key={group}
            group={group}
            settings={settings.filter((s) => s.group === group)}
            value={value}
            setValue={setValue}
            isDirty={isDirty}
            editable={isAdmin && writeCapability.available}
          />
        ))}
      </div>

      {isAdmin && dirty && (
        <Card className="footer-bar">
          <CardBody>
            <Pill tone="warn">
              {dirtyKeys.length} pending change{dirtyKeys.length === 1 ? "" : "s"}
            </Pill>
            <div className="spacer" />
            <button type="button" className="btn btn-ghost" onClick={() => setEdits({})}>
              Discard
            </button>
            <button type="button" className="btn" disabled={writeMutation.isPending} onClick={() => writeMutation.mutate()}>
              {writeMutation.isPending ? "Writing…" : "Write to compose file"}
            </button>
          </CardBody>
        </Card>
      )}

      {!dirty && serverPendingCount > 0 && (
        <Banner tone="warn">
          {serverPendingCount} setting{serverPendingCount === 1 ? "" : "s"} written to the compose file but not yet
          applied. From the host directory containing your compose file (relative bind paths resolve from there), run:
          <CodeWell>{configQuery.data!.manualCommand}</CodeWell>
        </Banner>
      )}
    </>
  );
}

function GroupCard({
  group,
  settings,
  value,
  setValue,
  isDirty,
  editable,
}: {
  group: string;
  settings: ConfigSetting[];
  value: (s: ConfigSetting) => string;
  setValue: (key: string, v: string) => void;
  isDirty: (s: ConfigSetting) => boolean;
  editable: boolean;
}) {
  const allReadOnly = settings.every((s) => s.readOnly);
  const useGrid = group === "gameplay";
  const [revealed, setRevealed] = useState<Record<string, boolean>>({});

  const body = settings.map((s) => {
    // Read-only booleans render as status rows rather than misleading controls.
    if (s.readOnly && s.type === "boolean") {
      return (
        <div className="kv-row" key={s.key}>
          <span className="k">{s.key.replace(/_ENABLED$/, "")}</span>
          <Pill tone={s.effectiveValue === true ? "ok" : "idle"}>{s.effectiveValue === true ? "Enabled" : "Disabled"}</Pill>
        </div>
      );
    }

    const isSecret = /password/i.test(s.key);
    const dirty = isDirty(s);
    const hint = dirty ? (
      <span className="field-hint warn">modified — will be written to compose (default {s.default || "empty"})</span>
    ) : s.pending ? (
      <span className="field-hint warn">written, not yet applied — server is running with {s.effectiveValue || "empty"}</span>
    ) : s.readOnly ? (
      <span className="field-hint">read-only in this deployment</span>
    ) : value(s) === String(s.default) ? (
      <span className="field-hint">default</span>
    ) : null;

    if (SELECT_OPTIONS[s.key]) {
      const options = SELECT_OPTIONS[s.key];
      return (
        <div className={`field${dirty ? " modified" : ""}`} key={s.key}>
          <label htmlFor={`cfg-${s.key}`}>{s.key}</label>
          <select
            id={`cfg-${s.key}`}
            className="input"
            value={value(s)}
            disabled={!editable || !s.editable}
            onChange={(e) => setValue(s.key, e.target.value)}
          >
            {options.map((o) => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
          </select>
          {hint}
        </div>
      );
    }

    if (s.type === "boolean") {
      return (
        <div className={`field${dirty ? " modified" : ""}`} key={s.key}>
          <label htmlFor={`cfg-${s.key}`}>{s.key}</label>
          <select
            id={`cfg-${s.key}`}
            className="input"
            value={value(s)}
            disabled={!editable || !s.editable}
            onChange={(e) => setValue(s.key, e.target.value)}
          >
            <option value="true">Enabled</option>
            <option value="false">Disabled</option>
          </select>
          {hint}
        </div>
      );
    }

    if (isSecret) {
      const shown = revealed[s.key] ?? false;
      return (
        <div className={`field${dirty ? " modified" : ""}`} key={s.key}>
          <label htmlFor={`cfg-${s.key}`}>{s.key}</label>
          <div style={{ display: "flex", gap: 8 }}>
            <input
              id={`cfg-${s.key}`}
              className="input input-mono"
              type={shown ? "text" : "password"}
              value={value(s) === "•••" ? "" : value(s)}
              placeholder="unchanged — enter a new value"
              readOnly={!editable || !s.editable}
              onChange={(e) => setValue(s.key, e.target.value)}
              style={{ flex: 1 }}
            />
            <button type="button" className="btn btn-sm btn-ghost" onClick={() => setRevealed((r) => ({ ...r, [s.key]: !shown }))}>
              {shown ? "Hide" : "Show"}
            </button>
          </div>
          {dirty ? hint : <span className="field-hint">write-only · the current value is never returned</span>}
        </div>
      );
    }

    return (
      <div className={`field${dirty ? " modified" : ""}`} key={s.key}>
        <label htmlFor={`cfg-${s.key}`}>{s.key}</label>
        <input
          id={`cfg-${s.key}`}
          className={`input${s.type === "number" || s.type === "integer" ? " input-mono" : ""}`}
          type={s.type === "number" || s.type === "integer" ? "number" : "text"}
          step={s.type === "integer" ? "1" : s.type === "number" ? "0.1" : undefined}
          value={value(s)}
          readOnly={!editable || !s.editable}
          style={(s.type === "number" || s.type === "integer") && !useGrid ? { width: 120 } : undefined}
          onChange={(e) => setValue(s.key, e.target.value)}
        />
        {hint}
      </div>
    );
  });

  return (
    <Card>
      <CardHead title={group.replace(/(^|-)(\w)/g, (_, lead: string, char: string) => `${lead ? " " : ""}${char.toUpperCase()}`)} hint={allReadOnly ? "read-only" : undefined} />
      <CardBody
        className={useGrid ? undefined : undefined}
        style={useGrid ? undefined : { display: "flex", flexDirection: "column", gap: "var(--space-3)" }}
      >
        {useGrid ? <div className="field-grid">{body}</div> : body}
        {allReadOnly && (
          <span className="field-hint">These values are displayed for status and cannot be changed through this editor.</span>
        )}
      </CardBody>
    </Card>
  );
}

// ---------------- raw ini ----------------

function RawIniTab() {
  const rawQuery = useQuery({ queryKey: ["config", "raw"], queryFn: () => api.config.raw() });

  return (
    <Card>
      <CardHead title="PalWorldSettings.ini" hint="read-only · regenerated on every boot" />
      {rawQuery.isError ? (
        <CardBody>
          <Banner tone="warn">Couldn't read the live ini file.</Banner>
        </CardBody>
      ) : rawQuery.isLoading ? (
        <CardBody>
          <span className="skel skel-text" style={{ width: "100%", height: 80 }} />
        </CardBody>
      ) : (
        <CardBody flush>
          <pre className="raw-ini">{rawQuery.data}</pre>
        </CardBody>
      )}
    </Card>
  );
}
