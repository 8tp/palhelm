export type DiffKind = "add" | "modify" | "delete";

export interface DiffItem {
  kind: DiffKind;
  text: string;
}

const CLASS_BY_KIND: Record<DiffKind, string> = { add: "add", modify: "chg", delete: "rem" };
const PREFIX_BY_KIND: Record<DiffKind, string> = { add: "+", modify: "~", delete: "−" };

export function DiffList({ items }: { items: DiffItem[] }) {
  return (
    <div className="diff-list">
      {items.map((item, i) => (
        <div key={i} className={CLASS_BY_KIND[item.kind]}>
          {PREFIX_BY_KIND[item.kind]} {item.text}
        </div>
      ))}
    </div>
  );
}
