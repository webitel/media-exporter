-- DROP SCHEMA media_exporter;

CREATE SCHEMA media_exporter;

-- DROP TYPE media_exporter."export_status";

CREATE TYPE media_exporter."export_status" AS ENUM (
  'pending',
  'processing',
  'done',
  'failed');

-- DROP SEQUENCE media_exporter.export_history_id_seq;

CREATE SEQUENCE media_exporter.export_history_id_seq
  INCREMENT BY 1
  MINVALUE 1
  MAXVALUE 9223372036854775807
  START 1
  CACHE 1
  NO CYCLE;

-- Permissions

ALTER SEQUENCE media_exporter.export_history_id_seq;
GRANT ALL ON SEQUENCE media_exporter.export_history_id_seq;
-- media_exporter.pdf_export_history definition

-- Drop table

-- DROP TABLE media_exporter.pdf_export_history;

CREATE TABLE media_exporter.pdf_export_history (
                                                 id int8 DEFAULT nextval('media_exporter.export_history_id_seq'::regclass) NOT NULL,
                                                 "name" varchar NOT NULL,
                                                 file_id int8 NULL,
                                                 mime varchar NOT NULL,
                                                 uploaded_at int8 NOT NULL,
                                                 updated_at int8 NOT NULL,
                                                 uploaded_by int8 NULL,
                                                 updated_by int8 NULL,
                                                 status media_exporter."export_status" DEFAULT 'pending'::media_exporter.export_status NOT NULL,
                                                 agent_id int8 NOT NULL
);
CREATE INDEX pdf_export_history_file_id_index ON media_exporter.pdf_export_history USING btree (file_id);
CREATE UNIQUE INDEX pdf_export_history_id_uindex ON media_exporter.pdf_export_history USING btree (id);

-- Permissions

ALTER TABLE media_exporter.pdf_export_history;
GRANT ALL ON TABLE media_exporter.pdf_export_history;


-- media_exporter.pdf_export_history foreign keys

ALTER TABLE media_exporter.pdf_export_history ADD CONSTRAINT pdf_export_history_cc_agent_id_fk FOREIGN KEY (agent_id) REFERENCES call_center.cc_agent(id) ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE media_exporter.pdf_export_history ADD CONSTRAINT pdf_export_history_files_id_fk FOREIGN KEY (file_id) REFERENCES "storage".files(id) ON DELETE CASCADE;
ALTER TABLE media_exporter.pdf_export_history ADD CONSTRAINT pdf_export_history_wbt_user_id_fk FOREIGN KEY (updated_by) REFERENCES directory.wbt_user(id) ON DELETE SET NULL DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE media_exporter.pdf_export_history ADD CONSTRAINT pdf_export_history_wbt_user_id_fk_2 FOREIGN KEY (uploaded_by) REFERENCES directory.wbt_user(id) ON DELETE SET NULL DEFERRABLE INITIALLY DEFERRED;




-- Permissions

GRANT ALL ON SCHEMA media_exporter;
