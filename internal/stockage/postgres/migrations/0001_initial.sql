create extension if not exists citext;

create table joueurs (
    id                    uuid primary key,
    pseudo                citext not null unique,
    elo                   integer not null default 1200,
    elo_max               integer not null default 1200,
    nb_parties_classees   integer not null default 0,
    nb_victoires_classees integer not null default 0,
    cree_le               timestamptz not null default now(),
    vu_le                 timestamptz not null default now(),
    constraint joueurs_pseudo_longueur check (char_length(pseudo) between 3 and 20)
);

create index joueurs_classement on joueurs (elo desc, id) where nb_parties_classees >= 10;

create table identites (
    fournisseur text not null,
    sujet       text not null,
    joueur_id   uuid not null references joueurs (id) on delete cascade,
    email       text,
    cree_le     timestamptz not null default now(),
    primary key (fournisseur, sujet)
);

create index identites_joueur on identites (joueur_id);

create table parties (
    id              uuid primary key,
    type            text not null check (type in ('classee', 'privee')),
    options         jsonb not null,
    statut          text not null check (statut in ('en_cours', 'terminee', 'annulee')),
    cause           text check (cause in ('score', 'forfait')),
    equipe_gagnante smallint check (equipe_gagnante in (0, 1)),
    score_a         integer,
    score_b         integer,
    debut           timestamptz not null,
    fin             timestamptz
);

create index parties_en_cours on parties (debut) where statut = 'en_cours';

create table participants (
    partie_id uuid not null references parties (id) on delete cascade,
    siege     smallint not null check (siege between 0 and 3),
    joueur_id uuid references joueurs (id) on delete set null,
    pseudo    text not null,
    bot       boolean not null default false,
    elo_avant integer,
    elo_apres integer,
    abandon   boolean not null default false,
    primary key (partie_id, siege)
);

create index participants_joueur on participants (joueur_id, partie_id);

create table evenements (
    partie_id  uuid not null references parties (id) on delete cascade,
    sequence   bigint not null,
    type       text not null,
    donnees    jsonb not null,
    horodatage timestamptz not null,
    primary key (partie_id, sequence)
);
