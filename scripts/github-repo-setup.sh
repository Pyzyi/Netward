#!/bin/sh
# Sets the GitHub "About" box, topics and features of the repository, turns on
# private vulnerability reporting and protects release tags.
# Run once on a computer with the GitHub CLI, logged in as the owner:  sh scripts/github-repo-setup.sh
set -e
repo=Pyzyi/netzwart

gh repo edit "$repo" \
  --description "Self-hosted home network monitor: see every device, who is online, new devices, outages and security issues. No cloud. ZimaOS, CasaOS, Unraid, TrueNAS, Docker." \
  --homepage "https://github.com/Pyzyi/netzwart#readme" \
  --enable-discussions \
  --enable-issues

gh repo edit "$repo" \
  --add-topic self-hosted --add-topic selfhosted --add-topic homelab \
  --add-topic network-monitoring --add-topic network-scanner --add-topic device-discovery \
  --add-topic home-network --add-topic presence-detection --add-topic home-assistant \
  --add-topic docker --add-topic zimaos --add-topic casaos --add-topic unraid --add-topic truenas \
  --add-topic fritzbox --add-topic security --add-topic golang --add-topic svelte

# Security reports go privately to the maintainer (SECURITY.md).
gh api -X PUT "repos/$repo/private-vulnerability-reporting" >/dev/null

# Release tags (v*) cannot be moved or deleted: a moved tag would publish
# other code under an existing version. Creating them stays open, because the
# release job creates the tag; CI only releases tags on commits in main.
if ! gh api "repos/$repo/rulesets" --jq '.[].name' | grep -qx 'Release tags'; then
  gh api -X POST "repos/$repo/rulesets" --input - >/dev/null <<'JSON'
{
  "name": "Release tags",
  "target": "tag",
  "enforcement": "active",
  "conditions": { "ref_name": { "include": ["refs/tags/v*"], "exclude": [] } },
  "rules": [ { "type": "update" }, { "type": "deletion" } ],
  "bypass_actors": [ { "actor_id": 5, "actor_type": "RepositoryRole", "bypass_mode": "always" } ]
}
JSON
fi

echo "Done. Upload docs/social-preview.png under Settings → General → Social preview (only possible in the browser)."
