import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router";
import { api } from "../../api/client";
import { ApiRequestError, type GuildDetail } from "../../api/types";
import { formatDuration, formatRelativeToNow } from "../../app/format";
import { worldToGame } from "../../app/mapTransform";
import { Banner } from "../../components/Banner";
import { Card, CardBody, CardHead } from "../../components/Card";
import { EmptyState } from "../../components/EmptyState";
import { PalIcon } from "../../components/PalIcon";
import { Pill } from "../../components/Pill";
import { palExplorerHref, palOwnerSummary, palSpecimenLabels } from "../pals/palExplorer";
import "./Guilds.css";

export default function GuildsRoute() {
  const { guildId } = useParams();
  const listQuery = useQuery({ queryKey: ["guilds"], queryFn: () => api.guilds.list(), enabled: !guildId });
  const detailQuery = useQuery({ queryKey: ["guilds", "detail", guildId], queryFn: () => api.guilds.detail(guildId ?? ""), enabled: Boolean(guildId) });
  const pending = guildId ? detailQuery.isPending : listQuery.isPending;
  const failed = guildId ? detailQuery.isError : listQuery.isError;
  const notFound = detailQuery.error instanceof ApiRequestError && detailQuery.error.status === 404;

  return (
    <main className="content guilds-page">
      <div className="page-head guilds-head">
        <div>
          <h1>{guildId ? detailQuery.data?.name || "Guild detail" : "Guilds"}</h1>
          <span className="sub">current save roster · bases · known members</span>
        </div>
        {guildId && <Link className="btn btn-sm btn-ghost" to="/guilds">All guilds</Link>}
      </div>

      {notFound ? (
        <Card><CardBody><EmptyState title="Guild not found" description="It may have disbanded or changed in the latest parsed save." /></CardBody></Card>
      ) : failed ? (
        <Banner tone="warn">Couldn't load guild data from the latest parsed save.</Banner>
      ) : pending ? (
        <Card><CardBody><span className="skel skel-text guilds-skeleton" /></CardBody></Card>
      ) : detailQuery.data ? (
        <GuildDetailView guild={detailQuery.data} />
      ) : (listQuery.data ?? []).length === 0 ? (
        <Card><CardBody><EmptyState title="No guilds found" description="Guilds appear after the world save has been parsed." /></CardBody></Card>
      ) : (
        <div className="guilds-grid">
          {(listQuery.data ?? []).map((item) => (
            <Card key={item.id}>
              <CardHead title={<Link to={`/guilds/${encodeURIComponent(item.id)}`}>{item.name || "Unnamed guild"}</Link>} hint={`${item.memberCount} members`} />
              <CardBody className="guild-card-body">
                <span>{item.bases.length} save-derived {item.bases.length === 1 ? "base" : "bases"}</span>
                <span>{item.members.length > 0 ? item.members.map((member) => member.name || "Unknown player").join(", ") : "No known member names"}</span>
              </CardBody>
            </Card>
          ))}
        </div>
      )}
    </main>
  );
}

function GuildDetailView({ guild }: { guild: GuildDetail }) {
  return (
    <>
      <Banner tone={guild.activity.analysisTruncated ? "warn" : "info"}>This is the current parsed save roster. The 30-day activity below is panel-observed and attributed to current membership; it does not reconstruct historical transfers.{guild.activity.analysisTruncated ? " Analysis hit its safety cap." : ""}</Banner>
      <div className="guilds-stats" aria-label="Guild summary">
        <Card><CardBody className="guild-stat"><span>Known members</span><strong>{guild.memberCount}</strong><small>{guild.members.filter((member) => member.online).length} online now</small></CardBody></Card>
        <Card><CardBody className="guild-stat"><span>Bases</span><strong>{guild.bases.length}</strong><small>{guild.bases.reduce((total, base) => total + base.palCount, 0)} linked base workers</small></CardBody></Card>
        <Card><CardBody className="guild-stat"><span>Associated Pals</span><strong>{guild.palCount}</strong><small>exact base or current-member association</small></CardBody></Card>
        <Card><CardBody className="guild-stat"><span>Observed activity · 30d</span><strong>{formatDuration(guild.activity.durationSec)}</strong><small>{guild.activity.sessionCount} sessions · {guild.activity.activePlayers} players</small></CardBody></Card>
      </div>
      <div className="guilds-detail-grid">
        <Card>
          <CardHead title="Members" hint="current save membership" />
          {guild.members.length === 0 ? <CardBody><EmptyState title="No linked players" description="The save has no player identities linked to this guild." /></CardBody> : (
            <CardBody flush className="guild-table-wrap">
              <table className="table"><thead><tr><th>Player</th><th>Level</th><th>Observed · 30d</th><th>Progress</th></tr></thead><tbody>
                {guild.members.map((member) => <tr key={member.uid}><td><strong><Link to={`/players?player=${encodeURIComponent(member.uid)}`}>{member.name || "Unknown player"}</Link></strong><small>{member.online ? <Pill tone="ok">Online</Pill> : `seen ${formatRelativeToNow(member.lastSeenAt)}`}</small></td><td className="num">{member.level}</td><td className="num">{formatDuration(member.observedDurationSec)}<small>{member.observedSessionCount} sessions</small></td><td>{member.paldeckUnlocked === null ? <span className="guild-muted">Unavailable</span> : <Link to={`/paldeck?player=${encodeURIComponent(member.uid)}`}>{member.paldeckUnlocked} save unlock counter</Link>}</td></tr>)}
              </tbody></table>
            </CardBody>
          )}
        </Card>
        <Card>
          <CardHead title="Bases" hint="Palworld display coordinates" />
          {guild.bases.length === 0 ? <CardBody><EmptyState title="No bases found" description="No current base records are linked to this guild." /></CardBody> : (
            <CardBody flush className="guild-table-wrap">
              <table className="table"><thead><tr><th>Base</th><th>Level</th><th>Pals</th><th>Location</th></tr></thead><tbody>
                {guild.bases.map((base, index) => {
                  const game = base.location ? worldToGame(base.location.x, base.location.y) : null;
                  return <tr key={base.id}><td>Base {index + 1}</td><td className="num">{base.level}</td><td className="num">{base.palCount}</td><td>{game ? <Link to={`/map?x=${game.x}&y=${game.y}`}>{game.x}, {game.y}</Link> : <span className="guild-muted">Unavailable</span>}</td></tr>;
                })}
              </tbody></table>
            </CardBody>
          )}
        </Card>
      </div>
      <Card>
        <CardHead title="Associated Pals" hint={`${guild.pals.length} returned${guild.palsTruncated ? " · bounded result truncated" : ""}`}>
          <Link to={palExplorerHref({ placement: "base" })}>Open Pal explorer</Link>
        </CardHead>
        {guild.pals.length === 0 ? <CardBody><EmptyState title="No associated Pals" description="No Pal currently joins a guild base or current member owner." /></CardBody> : (
          <CardBody className="guild-pal-grid">
            {guild.pals.map((pal) => {
              const specimen = palSpecimenLabels(pal);
              return <article key={pal.instanceId} className="guild-pal">
                <PalIcon characterId={pal.characterId} displayName={pal.displayName} />
                <div><strong>{pal.displayName}</strong><small>Lv {pal.level} · {specimen.length ? specimen.map((label) => label === "Boss" ? "◆ Boss" : label).join(" · ") : "Standard"}</small><small>{pal.association === "guild_base" ? "Exact guild base" : palOwnerSummary(pal)}</small></div>
                <Link to={palExplorerHref({ q: pal.displayName, placement: pal.association === "guild_base" ? "base" : "" })}>Roster</Link>
              </article>;
            })}
          </CardBody>
        )}
      </Card>
    </>
  );
}
