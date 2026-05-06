#!/bin/bash
set -e
cd /usr/share/GeoIP/
sudo wget -N https://mailfud.org/geoip-legacy/GeoIP.dat.gz
sudo gunzip -f GeoIP.dat.gz
