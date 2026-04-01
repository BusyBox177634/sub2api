package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

type UsageLogDetail struct {
	ent.Schema
}

func (UsageLogDetail) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "usage_log_details"},
	}
}

func (UsageLogDetail) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("usage_log_id").
			Unique(),
		field.String("request_payload_json").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("response_payload_json").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Int("request_payload_bytes").
			Optional().
			Nillable(),
		field.Int("response_payload_bytes").
			Optional().
			Nillable(),
		field.Bool("request_truncated").
			Default(false),
		field.Bool("response_truncated").
			Default(false),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (UsageLogDetail) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("usage_log", UsageLog.Type).
			Ref("detail").
			Field("usage_log_id").
			Required().
			Unique(),
	}
}
