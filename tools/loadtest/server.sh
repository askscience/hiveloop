#!/bin/sh
# Runs on the server. Isolated test copy of Hiveloop for load tests.
#   sh server.sh start <admin-password> [address]      test copy + relay + monitor (10 min)
#   sh server.sh report                                 summary of the monitor samples
#   sh server.sh stop                                   stop everything and delete the test data
# The test copy has its own empty database in ~/hiveloop-load/data, listens on 127.0.0.1:18091
# (reachable through the relay on <address>:18092) and never touches the real Hiveloop.
# HIVELOOP_BIN can point to the binary to copy (default: ~/hiveloop/hiveloop).
set -e
D=$HOME/hiveloop-load
pids() { for p in /proc/[0-9]*; do case $(readlink $p/exe 2>/dev/null) in "$1") echo ${p#/proc/};; esac; done; }

case "$1" in
start)
  [ -n "$2" ] || { echo "usage: server.sh start <admin-password> [address]"; exit 1; }
  IP=${3:-$(hostname -I 2>/dev/null | awk '{print $1}')}
  [ -n "$IP" ] || { echo "pass the address the load generator will connect to"; exit 1; }
  mkdir -p "$D/data"
  cp "${HIVELOOP_BIN:-$HOME/hiveloop/hiveloop}" "$D/hiveloop-test"   # own copy: updates never restart it
  cd "$D"
  HIVELOOP_ADDR=127.0.0.1:18091 HIVELOOP_DATA="$D/data" HIVELOOP_ADMIN_PASS="$2" HIVELOOP_SECURE_COOKIE=0 \
    HIVELOOP_AI_PROVIDER=openai HIVELOOP_AI_URL=http://127.0.0.1:9 setsid nohup ./hiveloop-test > test.log 2>&1 < /dev/null &
  setsid nohup ./relay "$IP:18092" 127.0.0.1:18091 > relay.log 2>&1 < /dev/null &
  sleep 3
  T=$(pids "$D/hiveloop-test"); P=$(pids "$(readlink -f "${HIVELOOP_BIN:-$HOME/hiveloop/hiveloop}")")
  setsid nohup sh ./mon.sh "$T" "$P" 600 > monitor.csv 2> mon.err < /dev/null &
  echo "test copy pid $T (real Hiveloop pid $P) — load URL: http://$IP:18092"
  ;;
report)
  awk -F, 'NR>1 && $2>0 {n++; tc+=$2; if($2>mt)mt=$2; kc+=$5; if($5>mk)mk=$5; b+=$6; if($6>mb)mb=$6; if($7>mr)mr=$7; if(mn==0||$9<mn)mn=$9; if($10>ms)ms=$10; if($11>ml)ml=$11; tx+=$15; if($15>mx)mx=$15}
    END{ if(!n){print "no samples yet"; exit}
      printf "Hiveloop CPU avg %d%% max %d%% (of one core)\nproxy CPU avg %d%% max %d%%\nsystem busy avg %d%% max %d%%\nHiveloop RAM max %d MB, free RAM min %d MB, swap max %d MB, load max %s\nnetwork out avg %d KB/s max %d KB/s\n", tc/n, mt, kc/n, mk, b/n, mb, mr/1024, mn, ms, ml, tx/n, mx }' "$D/monitor.csv"
  ;;
stop)
  for x in "$D/hiveloop-test" "$D/relay"; do for p in $(pids "$x"); do kill "$p" || true; done; done
  for p in /proc/[0-9]*; do case "$(tr '\0' ' ' < $p/cmdline 2>/dev/null)" in *"./mon.sh"*) kill ${p#/proc/} 2>/dev/null || true;; esac; done
  sleep 1
  rm -rf "$HOME/hiveloop-load"
  echo "stopped, test data deleted"
  ;;
*) echo "usage: server.sh start <admin-password> [tailnet-ip] | report | stop"; exit 1;;
esac
