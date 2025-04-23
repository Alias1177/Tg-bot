CREATE TABLE If NOT EXISTS users  (
    id serial primary key ,
    email text not null,
    country varchar not null
)