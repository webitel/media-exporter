create index if not exists pdf_export_history_agent_id_uploaded_at_index
  on media_exporter.pdf_export_history (agent_id, uploaded_at);
