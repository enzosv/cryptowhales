CREATE TABLE whale (
	whale_id int PRIMARY KEY GENERATED ALWAYS AS IDENTITY,
	blockchain varchar(16) NOT NULL,
	address varchar(64) NOT NULL,
	owner varchar(64) NULL,
	owner_type varchar(32) NOT NULL,
	is_contract bool NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT NOW(),
	updated_at timestamptz,
	CONSTRAINT ux_blockchain_address UNIQUE (blockchain, address)
);

CREATE TABLE balance (
	balance_id int PRIMARY KEY GENERATED ALWAYS AS IDENTITY,
	whale_id int NOT NULL REFERENCES whale(whale_id),
	value numeric NOT NULL,
	created_at timestamptz NOT NULL DEFAULT NOW(),
	symbol varchar(8) NOT NULL
);

CREATE FUNCTION trigger_set_updated()
    RETURNS trigger
    LANGUAGE plpgsql
AS $function$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$function$;

CREATE TRIGGER set_timestamp 
BEFORE UPDATE ON whale 
FOR EACH ROW EXECUTE FUNCTION trigger_set_updated();

CREATE INDEX created_at_idx ON balance USING btree (created_at);
CREATE INDEX symbol_idx ON balance USING btree (symbol);
CREATE INDEX value_idx ON balance USING btree (value);
CREATE INDEX whale_id_idx ON balance USING btree (whale_id);

CREATE INDEX blockchain_idx ON whale USING btree (blockchain);
CREATE INDEX is_contract_idx ON whale USING btree (is_contract);
CREATE INDEX owner_type_idx ON whale USING btree (owner_type);