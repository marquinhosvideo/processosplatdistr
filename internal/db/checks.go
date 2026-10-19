package db

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// MinServerVersion é a versão mínima do MySQL: o CHECK só é aplicado a partir
// do 8.0.16 (§3.1).
var MinServerVersion = [3]int{8, 0, 16}

// CheckVersion lê VERSION() e recusa servidores anteriores a MinServerVersion
// e o MariaDB, cujo CHECK, JSON e colunas geradas divergem do MySQL 8.
func CheckVersion(ctx context.Context, q Q) (string, error) {
	var v string
	if err := q.QueryRowContext(ctx, "SELECT VERSION()").Scan(&v); err != nil {
		return "", fmt.Errorf("ler a versão do MySQL: %w", err)
	}
	return v, checkVersion(v)
}

func checkVersion(v string) error {
	if strings.Contains(strings.ToLower(v), "mariadb") {
		return fmt.Errorf("servidor %q é MariaDB; a aplicação exige MySQL >= %s", v, minVersionString())
	}
	got, ok := parseVersion(v)
	if !ok {
		return fmt.Errorf("versão do MySQL não reconhecida: %q", v)
	}
	for i := range got {
		if got[i] != MinServerVersion[i] {
			if got[i] < MinServerVersion[i] {
				return fmt.Errorf("MySQL %s não é suportado: a aplicação exige >= %s (CHECK constraints)", v, minVersionString())
			}
			return nil
		}
	}
	return nil
}

// parseVersion lê "8.0.36-0ubuntu0.24.04.1" como [8 0 36].
func parseVersion(v string) ([3]int, bool) {
	var out [3]int
	core, _, _ := strings.Cut(v, "-")
	parts := strings.Split(core, ".")
	if len(parts) < 3 {
		return out, false
	}
	for i := range out {
		n, err := strconv.Atoi(parts[i])
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

func minVersionString() string {
	return fmt.Sprintf("%d.%d.%d", MinServerVersion[0], MinServerVersion[1], MinServerVersion[2])
}

// CheckMaxAllowedPacket exige @@max_allowed_packet >= min. Com
// FILES_BACKEND=mysql, um anexo de FILES_MAX_BYTES precisa caber num pacote,
// com 1 MB de folga para o resto do INSERT (§10.3).
func CheckMaxAllowedPacket(ctx context.Context, q Q, min int64) error {
	var got int64
	if err := q.QueryRowContext(ctx, "SELECT @@max_allowed_packet").Scan(&got); err != nil {
		return fmt.Errorf("ler @@max_allowed_packet: %w", err)
	}
	if got < min {
		return fmt.Errorf("@@max_allowed_packet do MySQL é %d bytes, menor que os %d exigidos por FILES_MAX_BYTES + 1 MB; aumente max_allowed_packet no servidor ou reduza FILES_MAX_BYTES", got, min)
	}
	return nil
}
