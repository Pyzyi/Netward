#!/bin/sh
# Sets the GitHub "About" box, topics and features of the repository.
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

echo "Done. Upload docs/social-preview.png under Settings → General → Social preview (only possible in the browser)."
