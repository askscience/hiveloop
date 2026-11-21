#!/bin/sh
# Samples every 2 s: CPU of the test copy and of the real Hiveloop, sshd and tailscaled, RSS,
# free memory, swap, load and network. Usage: mon.sh <test-pid> <prod-pid> <seconds> > out.csv
T=$1; P=$2; N=$(( $3 / 2 ))
ticks() { awk '{print $14+$15}' /proc/$1/stat 2>/dev/null || echo 0; }
rss() { awk '/VmRSS/{print $2}' /proc/$1/status 2>/dev/null || echo 0; }
cpu_all() { awk '/^cpu /{print $2+$3+$4+$5+$6+$7+$8, $5+$6}' /proc/stat; }
# total CPU ticks of all processes with a given name (sshd for the test tunnel, tailscaled for real users)
byname() { for f in /proc/[0-9]*/stat; do read -r l < "$f" 2>/dev/null || continue; case "$l" in *"($1)"*) echo "$l";; esac; done | awk '{s+=$14+$15} END{print s+0}'; }
net() { awk -v i="$1:" '$1==i{print $2, $10}' /proc/net/dev; }
echo "t,test_cpu_pct,prod_cpu_pct,ssh_cpu_pct,tailscale_cpu_pct,sys_busy_pct,test_rss_kb,prod_rss_kb,mem_avail_mb,swap_used_mb,load1,eth_rx_kbps,eth_tx_kbps,ts_rx_kbps,ts_tx_kbps"
set -- $(cpu_all); A0=$1; I0=$2
t0=$(ticks $T); p0=$(ticks $P); s0=$(byname sshd); k0=$(byname tailscaled)
set -- $(net eth0); er0=$1; et0=$2
set -- $(net "${NETIF:-tailscale0}"); tr0=${1:-0}; tt0=${2:-0}
i=0
while [ $i -lt $N ]; do
  sleep 2; i=$((i+1))
  set -- $(cpu_all); A1=$1; I1=$2
  t1=$(ticks $T); p1=$(ticks $P); s1=$(byname sshd); k1=$(byname tailscaled)
  set -- $(net eth0); er1=$1; et1=$2
  set -- $(net "${NETIF:-tailscale0}"); tr1=${1:-0}; tt1=${2:-0}
  dA=$((A1-A0)); [ $dA -le 0 ] && dA=1
  # per-process CPU as % of ONE core (100 = one core fully busy; 4 threads = up to 400)
  hz=100
  tc=$(( (t1-t0) * 100 / (2*hz) )); pc=$(( (p1-p0) * 100 / (2*hz) ))
  sc=$(( (s1-s0) * 100 / (2*hz) )); kc=$(( (k1-k0) * 100 / (2*hz) ))
  busy=$(( 100 - (I1-I0) * 100 / dA ))
  ma=$(awk '/MemAvailable/{print int($2/1024)}' /proc/meminfo)
  su=$(awk '/SwapTotal/{t=$2} /SwapFree/{f=$2} END{print int((t-f)/1024)}' /proc/meminfo)
  l1=$(cut -d' ' -f1 /proc/loadavg)
  echo "$((i*2)),$tc,$pc,$sc,$kc,$busy,$(rss $T),$(rss $P),$ma,$su,$l1,$(( (er1-er0)/2048 )),$(( (et1-et0)/2048 )),$(( (tr1-tr0)/2048 )),$(( (tt1-tt0)/2048 ))"
  A0=$A1; I0=$I1; t0=$t1; p0=$p1; s0=$s1; k0=$k1; er0=$er1; et0=$et1; tr0=$tr1; tt0=$tt1
done
