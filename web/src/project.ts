// Where the project lives and who made it: shown in the footer of every page
// and in the About popover of the workspace views.
export const project = {
  name: "kubedojo",
  repository: "https://github.com/Mistral-valaise/kubedojo",
  author: { name: "Mistral N", url: "https://mistraln.de" },
};

/**
 * The name with the version the server reports: "kubedojo v0.5.0". A release
 * number gets its "v" when it lacks one; anything else ("dev", a commit) is
 * shown as it is. Without a version (still loading) it is the name alone.
 */
export function versionLabel(version: string | null | undefined): string {
  if (!version) return project.name;
  return `${project.name} ${/^\d/.test(version) ? `v${version}` : version}`;
}
