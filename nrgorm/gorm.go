package nrgorm

import (
	"fmt"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/newrelic/go-agent/v3/newrelic"
)

const (
	txnGormKey   = "newrelicTransaction"
	startTimeKey = "newrelicStartTime"
)

// SetTxnToGorm sets transaction to gorm settings, returns cloned DB
func SetTxnToGorm(txn newrelic.Transaction, db *gorm.DB) *gorm.DB {
	return db.Set(txnGormKey, txn)
}

// AddGormCallbacks adds callbacks to NewRelic, you should call SetTxnToGorm to make them work
func AddGormCallbacks(db *gorm.DB) {
	dialect := db.Dialect().GetName()
	var product newrelic.DatastoreProduct
	switch dialect {
	case "postgres":
		product = newrelic.DatastorePostgres
	case "mysql":
		product = newrelic.DatastoreMySQL
	case "sqlite3":
		product = newrelic.DatastoreSQLite
	case "mssql":
		product = newrelic.DatastoreMSSQL
	default:
		return
	}
	callbacks := newCallbacks(product)
	registerCallbacks(db, "transaction", callbacks)
	registerCallbacks(db, "create", callbacks)
	registerCallbacks(db, "query", callbacks)
	registerCallbacks(db, "update", callbacks)
	registerCallbacks(db, "delete", callbacks)
	registerCallbacks(db, "row_query", callbacks)
}

type callbacks struct {
	product newrelic.DatastoreProduct
}

func newCallbacks(product newrelic.DatastoreProduct) *callbacks {
	return &callbacks{product}
}

func (c *callbacks) beforeCreate(scope *gorm.Scope)   { c.before(scope) }
func (c *callbacks) afterCreate(scope *gorm.Scope)    { c.after(scope, "INSERT") }
func (c *callbacks) beforeQuery(scope *gorm.Scope)    { c.before(scope) }
func (c *callbacks) afterQuery(scope *gorm.Scope)     { c.after(scope, "SELECT") }
func (c *callbacks) beforeUpdate(scope *gorm.Scope)   { c.before(scope) }
func (c *callbacks) afterUpdate(scope *gorm.Scope)    { c.after(scope, "UPDATE") }
func (c *callbacks) beforeDelete(scope *gorm.Scope)   { c.before(scope) }
func (c *callbacks) afterDelete(scope *gorm.Scope)    { c.after(scope, "DELETE") }
func (c *callbacks) beforeRowQuery(scope *gorm.Scope) { c.before(scope) }
func (c *callbacks) afterRowQuery(scope *gorm.Scope)  { c.after(scope, "") }

func (c *callbacks) before(scope *gorm.Scope) {
	txn, ok := scope.Get(txnGormKey)
	if !ok {
		return
	}
	ttxn := txn.(newrelic.Transaction)
	scope.Set(startTimeKey, ttxn.StartSegmentNow())
}

func (c *callbacks) after(scope *gorm.Scope, operation string) {
	startTime, ok := scope.Get(startTimeKey)
	if !ok {
		return
	}
	if operation == "" {
		operation = strings.ToUpper(strings.Split(scope.SQL, " ")[0])
	}
	segmentBuilder(
		startTime.(newrelic.SegmentStartTime),
		c.product,
		scope.SQL,
		operation,
		scope.TableName(),
	).End()

	// gorm wraps insert&update into transaction automatically
	// add another segment for commit/rollback in such case
	if _, ok := scope.InstanceGet("gorm:started_transaction"); !ok {
		scope.Set(startTimeKey, nil)
		return
	}
	txn, _ := scope.Get(txnGormKey)
	ttxn := txn.(newrelic.Transaction)
	scope.Set(startTimeKey, ttxn.StartSegmentNow())
}

func (c *callbacks) commitOrRollback(scope *gorm.Scope) {
	startTime, ok := scope.Get(startTimeKey)
	if !ok || startTime == nil {
		return
	}

	segmentBuilder(
		startTime.(newrelic.SegmentStartTime),
		c.product,
		"",
		"COMMIT/ROLLBACK",
		scope.TableName(),
	).End()
}

func registerCallbacks(db *gorm.DB, name string, c *callbacks) {
	beforeName := fmt.Sprintf("newrelic:%v_before", name)
	afterName := fmt.Sprintf("newrelic:%v_after", name)
	gormCallbackName := fmt.Sprintf("gorm:%v", name)
	// gorm does some magic, if you pass CallbackProcessor here - nothing works
	switch name {
	case "create":
		db.Callback().Create().Before(gormCallbackName).Register(beforeName, c.beforeCreate)
		db.Callback().Create().After(gormCallbackName).Register(afterName, c.afterCreate)
		db.Callback().Create().
			After("gorm:commit_or_rollback_transaction").
			Register(fmt.Sprintf("newrelic:commit_or_rollback_transaction_%v", name), c.commitOrRollback)
	case "query":
		db.Callback().Query().Before(gormCallbackName).Register(beforeName, c.beforeQuery)
		db.Callback().Query().After(gormCallbackName).Register(afterName, c.afterQuery)
	case "update":
		db.Callback().Update().Before(gormCallbackName).Register(beforeName, c.beforeUpdate)
		db.Callback().Update().After(gormCallbackName).Register(afterName, c.afterUpdate)
		db.Callback().Update().
			After("gorm:commit_or_rollback_transaction").
			Register(fmt.Sprintf("newrelic:commit_or_rollback_transaction_%v", name), c.commitOrRollback)
	case "delete":
		db.Callback().Delete().Before(gormCallbackName).Register(beforeName, c.beforeDelete)
		db.Callback().Delete().After(gormCallbackName).Register(afterName, c.afterDelete)
		db.Callback().Delete().
			After("gorm:commit_or_rollback_transaction").
			Register(fmt.Sprintf("newrelic:commit_or_rollback_transaction_%v", name), c.commitOrRollback)
	case "row_query":
		db.Callback().RowQuery().Before(gormCallbackName).Register(beforeName, c.beforeRowQuery)
		db.Callback().RowQuery().After(gormCallbackName).Register(afterName, c.afterRowQuery)
	}
}

type segment interface {
	End()
}

// create segment through function to be able to test it
var segmentBuilder = func(
	startTime newrelic.SegmentStartTime,
	product newrelic.DatastoreProduct,
	query string,
	operation string,
	collection string,
) segment {
	return &newrelic.DatastoreSegment{
		StartTime:          startTime,
		Product:            product,
		ParameterizedQuery: query,
		Operation:          operation,
		Collection:         collection,
	}
}
