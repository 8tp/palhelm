// Display label for a guild.
//
// Most guilds on a live server never set a name, so the raw save value is empty
// (or literally "Unnamed Guild"). Rather than show "Unnamed guild" over and over,
// borrow a member's name when we know one: the guild's admin/founder if the save
// identifies one, otherwise the first known member — e.g. "Ada's guild". With no
// known member names we keep the honest "Unnamed guild".
//
// Pure and structural on purpose: it takes anything shaped like a guild summary
// (a name, an optional admin uid, and a members array of uid+name), so both the
// guild list summary and the fuller guild detail can pass straight through.

export const UNNAMED_GUILD_LABEL = "Unnamed guild";

export interface GuildLabelMember {
  uid: string;
  name: string;
}

export interface GuildLabelInput {
  name?: string | null;
  adminUid?: string | null;
  members?: readonly GuildLabelMember[] | null;
}

/** A save name counts as "no real name" when it's blank or the default placeholder. */
function isUnnamed(name: string | null | undefined): boolean {
  const trimmed = (name ?? "").trim();
  return trimmed === "" || trimmed.toLowerCase() === "unnamed guild";
}

export function guildDisplayName(guild: GuildLabelInput): string {
  const name = (guild.name ?? "").trim();
  if (!isUnnamed(name)) return name;

  const named = (guild.members ?? []).filter((member) => (member.name ?? "").trim() !== "");
  if (named.length === 0) return UNNAMED_GUILD_LABEL;

  const admin = guild.adminUid ? named.find((member) => member.uid === guild.adminUid) : undefined;
  const chosen = admin ?? named[0];
  return `${chosen.name.trim()}'s guild`;
}
