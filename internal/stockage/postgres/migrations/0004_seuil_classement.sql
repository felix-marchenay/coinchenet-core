drop index joueurs_classement;
create index joueurs_classement on joueurs (elo desc, id) where nb_parties_classees >= 5;
