alter table parties drop constraint parties_type_check;
alter table parties add constraint parties_type_check check (type in ('classee', 'privee', 'avec_bots'));
