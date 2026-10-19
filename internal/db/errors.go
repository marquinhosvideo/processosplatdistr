package db

import (
	"errors"
	"strings"

	"github.com/go-sql-driver/mysql"
)

// Códigos de erro do MySQL usados pelos helpers.
const (
	ErrDupEntry          = 1062 // ER_DUP_ENTRY
	ErrNoSuchTable       = 1146 // ER_NO_SUCH_TABLE
	ErrRowIsReferenced   = 1451 // ER_ROW_IS_REFERENCED_2: DELETE/UPDATE do pai com filhos
	ErrNoReferencedRow   = 1452 // ER_NO_REFERENCED_ROW_2: INSERT/UPDATE do filho sem pai
	ErrCheckViolated     = 3819 // ER_CHECK_CONSTRAINT_VIOLATED
	ErrLockNowait        = 3572 // ER_LOCK_NOWAIT: FOR UPDATE NOWAIT com a linha travada
	errRowIsReferencedV1 = 1217 // ER_ROW_IS_REFERENCED (servidores antigos)
	errNoReferencedRowV1 = 1216 // ER_NO_REFERENCED_ROW (servidores antigos)
)

// Code devolve o número do erro do MySQL em err, ou 0.
func Code(err error) uint16 {
	var me *mysql.MySQLError
	if errors.As(err, &me) {
		return me.Number
	}
	return 0
}

// IsDuplicate informa se err é violação de chave única ou primária.
func IsDuplicate(err error) bool { return Code(err) == ErrDupEntry }

// DuplicateKey devolve o nome da chave única violada (ex.: "uq_people_email"),
// para o módulo distinguir qual regra foi quebrada.
func DuplicateKey(err error) (string, bool) {
	var me *mysql.MySQLError
	if !errors.As(err, &me) || me.Number != ErrDupEntry {
		return "", false
	}
	// "Duplicate entry 'x' for key 'tabela.uq_nome'" (o prefixo da tabela
	// existe desde o 8.0.19).
	const marker = "for key '"
	i := strings.LastIndex(me.Message, marker)
	if i < 0 {
		return "", true
	}
	key := strings.TrimSuffix(me.Message[i+len(marker):], "'")
	if j := strings.LastIndexByte(key, '.'); j >= 0 {
		key = key[j+1:]
	}
	return key, true
}

// IsFKViolation informa se err é violação de chave estrangeira, nos dois
// sentidos: filho sem pai ou pai ainda referenciado.
func IsFKViolation(err error) bool {
	switch Code(err) {
	case ErrRowIsReferenced, ErrNoReferencedRow, errRowIsReferencedV1, errNoReferencedRowV1:
		return true
	}
	return false
}

// IsCheckViolation informa se err é violação de CHECK constraint.
func IsCheckViolation(err error) bool { return Code(err) == ErrCheckViolated }

// IsLockNowait informa se um SELECT … FOR UPDATE NOWAIT encontrou a linha
// travada por outra transação (o recheck de papéis segue com os atuais).
func IsLockNowait(err error) bool { return Code(err) == ErrLockNowait }
