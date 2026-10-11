#!/bin/sh
# region: firewall
# Run on the VPS as root: sudo sh firewall.sh
# Everything that comes in is dropped, except SSH, HTTP and HTTPS from
# anywhere, and traffic from the K3s pod and Service networks, which
# reach the API server and other programs on the node. Port 6443 stays
# closed to the internet.
set -eu
ufw default deny incoming
ufw default allow outgoing
ufw allow 22/tcp
ufw allow 80/tcp
ufw allow 443/tcp
ufw allow from 10.42.0.0/16 to any
ufw allow from 10.43.0.0/16 to any
ufw --force enable
ufw status verbose
# endregion: firewall
