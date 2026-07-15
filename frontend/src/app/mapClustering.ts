export type ClusterMarkerKind = "player" | "base" | "worker";

/** A marker already projected into screen space. `value` retains the original entity/coordinates. */
export interface ClusterMarkerPoint<T> {
  key: string;
  kind: ClusterMarkerKind;
  layerId: string;
  x: number;
  y: number;
  value: T;
}

export type ClusterMarkerGroup<T> =
  | { type: "single"; key: string; x: number; y: number; member: ClusterMarkerPoint<T> }
  | { type: "cluster"; key: string; x: number; y: number; members: ClusterMarkerPoint<T>[] };

/**
 * Groups dense markers in screen space. Clusters never cross tile-layer or marker-kind
 * boundaries, and the selected marker is always emitted as a standalone marker so focus and
 * exact-coordinate access cannot be hidden by aggregation.
 *
 * Points are admitted in stable key order only when they remain within the radius of every
 * existing member. This caps cluster diameter and prevents an A-near-B-near-C chain from
 * percolating across a whole dense region when A and C are far apart.
 */
export function clusterMapMarkers<T>(
  points: readonly ClusterMarkerPoint<T>[],
  radiusPx = 48,
  selectedKey: string | null = null,
): ClusterMarkerGroup<T>[] {
  if (!Number.isFinite(radiusPx) || radiusPx <= 0) {
    return points.map((member) => ({ type: "single", key: member.key, x: member.x, y: member.y, member }));
  }

  const ordered = [...points].sort((left, right) => left.key.localeCompare(right.key));
  const radiusSquared = radiusPx * radiusPx;
  const components: Array<{ locked: boolean; members: ClusterMarkerPoint<T>[] }> = [];
  for (const point of ordered) {
    const component = point.key === selectedKey
      ? undefined
      : components.find((candidate) =>
          !candidate.locked &&
          candidate.members[0].kind === point.kind &&
          candidate.members[0].layerId === point.layerId &&
          candidate.members.every((member) => {
            const dx = member.x - point.x;
            const dy = member.y - point.y;
            return dx * dx + dy * dy <= radiusSquared;
          }));
    if (component) component.members.push(point);
    else components.push({ locked: point.key === selectedKey, members: [point] });
  }

  return components.map(({ members }): ClusterMarkerGroup<T> => {
    if (members.length === 1) {
      const member = members[0];
      return { type: "single", key: member.key, x: member.x, y: member.y, member };
    }
    return {
      type: "cluster",
      key: `cluster:${members[0].layerId}:${members[0].kind}:${members.map((member) => member.key).join("|")}`,
      x: members.reduce((sum, member) => sum + member.x, 0) / members.length,
      y: members.reduce((sum, member) => sum + member.y, 0) / members.length,
      members,
    };
  });
}
