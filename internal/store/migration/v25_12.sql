alter table media_exporter.pdf_export_history
  add dc bigint not null;

alter table media_exporter.pdf_export_history
  add constraint pdf_export_history_wbt_domain_dc_fk
    foreign key (dc) references directory.wbt_domain
      on delete cascade;



