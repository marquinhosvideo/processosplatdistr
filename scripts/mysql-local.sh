#!/usr/bin/env bash
# MySQL 8 local para desenvolvimento e testes de integração (SDD §9.2).
#
#   scripts/mysql-local.sh start    instala (se preciso), inicializa e sobe em 127.0.0.1:3306
#   scripts/mysql-local.sh stop     derruba o mysqld que este script subiu
#   scripts/mysql-local.sh status   0 se responde, 3 se parado
#
# Os dados ficam em .tmp/mysql (ignorado pelo git). O mysqld roda com
# --no-defaults: o /etc/mysql/my.cnf do pacote fixa user=mysql, que não
# escreve no checkout, e ignoraria o --user=root. O usuário app/app tem
# ALL ON *.*, porque os testes criam e apagam bancos. Um mysqld que já esteja
# respondendo é reaproveitado: o start nunca derruba o banco de outro processo.
set -euo pipefail

cd "$(dirname "$0")/.."

ROOT="$PWD/.tmp/mysql"
DATADIR="$ROOT/data"
SOCKET="$ROOT/mysqld.sock"
PIDFILE="$ROOT/mysqld.pid"
ERRLOG="$ROOT/error.log"
PORT="${MYSQL_LOCAL_PORT:-3306}"
WAIT_SECONDS="${MYSQL_LOCAL_WAIT:-60}"

log() { echo "mysql-local: $*" >&2; }
die() { log "ERRO: $*"; exit 1; }

find_bin() {
  local name=$1
  if command -v "$name" >/dev/null 2>&1; then
    command -v "$name"
  elif [ -x "/usr/sbin/$name" ]; then
    echo "/usr/sbin/$name"
  else
    return 1
  fi
}

# ping responde 0 se há um mysqld aceitando conexões (pelo socket deste
# script ou pela porta TCP).
ping() {
  local admin
  admin=$(find_bin mysqladmin) || return 1
  if [ -S "$SOCKET" ] && "$admin" --no-defaults --socket="$SOCKET" -uroot --connect-timeout=2 ping >/dev/null 2>&1; then
    return 0
  fi
  MYSQL_PWD=app "$admin" --no-defaults --protocol=TCP -h127.0.0.1 -P"$PORT" -uapp --connect-timeout=2 ping >/dev/null 2>&1
}

running_pid() {
  [ -f "$PIDFILE" ] || return 1
  local pid
  pid=$(cat "$PIDFILE" 2>/dev/null) || return 1
  [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null && echo "$pid"
}

install_mysql() {
  if ! command -v apt-get >/dev/null 2>&1; then
    die "mysqld não encontrado e apt-get indisponível; instale o MySQL 8 (>= 8.0.16) manualmente. Sem ele, os testes de integração são pulados."
  fi
  log "mysqld não encontrado; instalando mysql-server-8.0 via apt-get"
  # Impede o pacote de subir o serviço padrão em /var/lib/mysql e ocupar a 3306.
  local policy=/usr/sbin/policy-rc.d created_policy=0
  if [ ! -e "$policy" ]; then
    printf '#!/bin/sh\nexit 101\n' >"$policy" && chmod +x "$policy" && created_policy=1
  fi
  local ok=1
  { apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends mysql-server-8.0; } >&2 || ok=0
  [ "$created_policy" = 1 ] && rm -f "$policy"
  [ "$ok" = 1 ] || die "apt-get install mysql-server-8.0 falhou (rede ou repositório indisponível). Sem MySQL, os testes de integração são pulados."
}

cmd_start() {
  if ping; then
    log "mysqld já está respondendo em 127.0.0.1:$PORT; reaproveitando"
    ensure_app_user
    return 0
  fi

  local mysqld
  if ! mysqld=$(find_bin mysqld); then
    install_mysql
    mysqld=$(find_bin mysqld) || die "mysqld continua ausente depois da instalação"
  fi

  mkdir -p /var/run/mysqld "$ROOT"
  if [ ! -d "$DATADIR/mysql" ]; then
    log "inicializando o datadir em $DATADIR"
    rm -rf "$DATADIR"
    "$mysqld" --no-defaults --initialize-insecure --user=root --datadir="$DATADIR" --log-error="$ROOT/init.log" \
      || die "mysqld --initialize-insecure falhou; veja $ROOT/init.log"
  fi

  log "subindo mysqld em 127.0.0.1:$PORT (log em $ERRLOG)"
  # setsid + nohup: o mysqld sobrevive ao fim do shell que o iniciou.
  setsid nohup "$mysqld" --no-defaults --user=root --datadir="$DATADIR" --socket="$SOCKET" \
    --pid-file="$PIDFILE" --log-error="$ERRLOG" \
    --port="$PORT" --bind-address=127.0.0.1 --mysqlx=OFF --skip-log-bin \
    --default-time-zone=+00:00 --character-set-server=utf8mb4 --collation-server=utf8mb4_0900_ai_ci \
    --sql-require-primary-key=ON --max-allowed-packet=64M --innodb-flush-log-at-trx-commit=2 \
    </dev/null >/dev/null 2>&1 &

  local i
  for ((i = 0; i < WAIT_SECONDS * 2; i++)); do
    if [ -S "$SOCKET" ] && "$(find_bin mysqladmin)" --no-defaults --socket="$SOCKET" -uroot ping >/dev/null 2>&1; then
      ensure_app_user
      log "pronto: DSN de teste app:app@tcp(127.0.0.1:$PORT)/"
      return 0
    fi
    sleep 0.5
  done
  tail -n 20 "$ERRLOG" >&2 2>/dev/null || true
  die "mysqld não respondeu em ${WAIT_SECONDS}s; veja $ERRLOG"
}

# ensure_app_user cria o usuário dos testes. Só é possível pelo socket deste
# script (root sem senha); num mysqld alheio, apenas confere o acesso.
ensure_app_user() {
  local client
  client=$(find_bin mysql) || die "cliente mysql não encontrado"
  if [ -S "$SOCKET" ] && "$client" --no-defaults --socket="$SOCKET" -uroot -e 'SELECT 1' >/dev/null 2>&1; then
    "$client" --no-defaults --socket="$SOCKET" -uroot <<'SQL'
CREATE USER IF NOT EXISTS 'app'@'127.0.0.1' IDENTIFIED BY 'app';
GRANT ALL ON *.* TO 'app'@'127.0.0.1';
SQL
    return 0
  fi
  MYSQL_PWD=app "$client" --no-defaults --protocol=TCP -h127.0.0.1 -P"$PORT" -uapp -e 'SELECT 1' >/dev/null 2>&1 \
    || die "há um mysqld em 127.0.0.1:$PORT que não foi subido por este script e não aceita app/app"
}

cmd_stop() {
  local pid
  if ! pid=$(running_pid); then
    log "nenhum mysqld deste script em execução"
    return 0
  fi
  log "derrubando mysqld (pid $pid)"
  "$(find_bin mysqladmin)" --no-defaults --socket="$SOCKET" -uroot shutdown >/dev/null 2>&1 || kill "$pid" 2>/dev/null || true
  local i
  for ((i = 0; i < 60; i++)); do
    kill -0 "$pid" 2>/dev/null || { log "parado"; return 0; }
    sleep 0.5
  done
  die "mysqld (pid $pid) não parou em 30s"
}

cmd_status() {
  local pid
  if ping; then
    if pid=$(running_pid); then
      echo "rodando (pid $pid, 127.0.0.1:$PORT, dados em $DATADIR)"
    else
      echo "rodando (127.0.0.1:$PORT, mysqld externo a este script)"
    fi
    return 0
  fi
  echo "parado"
  return 3
}

case "${1:-}" in
  start) cmd_start ;;
  stop) cmd_stop ;;
  status) cmd_status ;;
  *)
    echo "uso: $0 start|stop|status" >&2
    exit 2
    ;;
esac
