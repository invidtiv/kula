#!/bin/bash

cd "$(dirname "$0")"
wget -nv -O roles/kula/files/kula.deb https://github.com/c0m4r/kula/releases/download/0.12.1/kula-0.12.1-amd64.deb
wget -nv -O roles/kula/files/kula.rpm https://github.com/c0m4r/kula/releases/download/0.12.1/kula-0.12.1-x86_64.rpm
