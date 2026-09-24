# themes fonts — valid `font_picker` handles

A `font_picker` value in theme settings is a **handle**: `<family>_<style><weight>`, where style
is `n` (normal) or `i` (italic) and weight is `1`–`9` (thin → black), e.g. `lato_n4`. The server
stores any string without checking it, and an unknown handle silently breaks the font on the
storefront — so this table is the only guard.

**Rule: write a handle only if it appears below exactly, character for character.** Never build
one by concatenating a family name, and never change a suffix unless that suffix is listed for
the family.

## Choosing

| Situation | What to do |
|---|---|
| A font is named | Find it below and write its handle verbatim |
| Named font not in the table | Say it isn't available here; point the merchant to the theme editor's font picker (it may offer more). You may suggest 2–3 listed fonts of the same kind (serif / sans / script / display) |
| Only a direction ("more elegant", "less formal") | Offer 2–3 candidates and wait for a pick — fonts have no ordered scale, so don't choose one yourself |
| Bolder / lighter / italic | Use a suffix listed in the "Other styles" column for that family (e.g. `montserrat_n7`); not listed → say so and point to the editor's font picker |
| The field's schema says `value_type: "json"` | It needs a full font object this table can't express — say so and point to the editor's font picker |

Theme settings take the handle string. A font field inside a card holds a font object — copy the
shape of its current value (see [setting-values.md](setting-values.md)).

Source: the public font library at shoplazza.dev → Theme → Settings → Fonts. All families are
Google Fonts.

## Fonts (family | default handle | other styles)

```
Abril Fatface         | abril_fatface_n4        | —
Amiri                 | amiri_n4                | n7 i4 i7
Archivo               | archivo_n4              | n1 n2 n3 n5 n6 n7 n8 n9 i1 i2 i3 i4 i5 i6 i7 i8 i9
Archivo Black         | archivo_black_n4        | —
Arimo                 | arimo_n4                | n5 n6 n7 i4 i5 i6 i7
Bakbak One            | bakbak_one_n4           | —
Cardo                 | cardo_n4                | n7 i4
Caudex                | caudex_n4               | n7 i4 i7
Concert One           | concert_one_n4          | —
Cormorant             | cormorant_n4            | n3 n5 n6 n7 i3 i4 i5 i6 i7
Cormorant Garamond    | cormorant_garamond_n4   | n3 n5 n6 n7 i3 i4 i5 i6 i7
Crimson Pro           | crimson_pro_n4          | n2 n3 n5 n6 n7 n8 n9 i2 i3 i4 i5 i6 i7 i8 i9
Didact Gothic         | didact_gothic_n4        | —
Eater                 | eater_n4                | —
Eczar                 | eczar_n4                | n5 n6 n7 n8
Exo 2                 | exo_2_n4                | n1 n2 n3 n5 n6 n7 n8 n9 i1 i2 i3 i4 i5 i6 i7 i8 i9
Fira Sans             | fira_sans_n4            | n1 n2 n3 n5 n6 n7 n8 n9 i1 i2 i3 i4 i5 i6 i7 i8 i9
Fjalla One            | fjalla_one_n4           | —
Frank Ruhl Libre      | frank_ruhl_libre_n4     | n3 n5 n7 n9
Heebo                 | heebo_n4                | n1 n2 n3 n5 n6 n7 n8 n9
Hind Siliguri         | hind_siliguri_n4        | n3 n5 n6 n7
Josefin Sans          | josefin_sans_n4         | n1 n2 n3 n5 n6 n7 i1 i2 i3 i4 i5 i6 i7
Jost                  | jost_n4                 | n1 n2 n3 n5 n6 n7 n8 n9 i1 i2 i3 i4 i5 i6 i7 i8 i9
Karla                 | karla_n4                | n2 n3 n5 n6 n7 n8 i2 i3 i4 i5 i6 i7 i8
Lato                  | lato_n4                 | n1 n3 n7 n9 i1 i3 i4 i7 i9
Lexend                | lexend_n4               | n1 n2 n3 n5 n6 n7 n8 n9
Lobster Two           | lobster_two_n4          | n7 i4 i7
Lora                  | lora_n4                 | n5 n6 n7 i4 i5 i6 i7
Lusitana              | lusitana_n4             | n7
Marcellus             | marcellus_n4            | —
Merriweather          | merriweather_n4         | n3 n7 n9 i3 i4 i7 i9
Modern Antiqua        | modern_antiqua_n4       | —
Monda                 | monda_n4                | n7
Montserrat            | montserrat_n4           | n1 n2 n3 n5 n6 n7 n8 n9 i1 i2 i3 i4 i5 i6 i7 i8 i9
Mukta Mahee           | mukta_mahee_n4          | n2 n3 n5 n6 n7 n8
Mulish                | mulish_n4               | n2 n3 n5 n6 n7 n8 n9 i2 i3 i4 i5 i6 i7 i8 i9
Noto Sans             | noto_sans_n4            | n7 i4 i7
Noto Serif            | noto_serif_n4           | n7 i4 i7
Nunito                | nunito_n4               | n2 n3 n5 n6 n7 n8 n9 i2 i3 i4 i5 i6 i7 i8 i9
Nunito Sans           | nunito_sans_n4          | n2 n3 n5 n6 n7 n8 n9 i2 i3 i4 i5 i6 i7 i8 i9
Open Sans             | open_sans_n4            | n3 n5 n6 n7 n8 i3 i4 i5 i6 i7 i8
Oranienbaum           | oranienbaum_n4          | —
Oswald                | oswald_n4               | n2 n3 n5 n6 n7
Overlock              | overlock_n4             | n7 n9 i4 i7 i9
Philosopher           | philosopher_n4          | n7 i4 i7
Playfair Display      | playfair_display_n4     | n5 n6 n7 n8 n9 i4 i5 i6 i7 i8 i9
Poly                  | poly_n4                 | i4
Poppins               | poppins_n4              | n1 n2 n3 n5 n6 n7 n8 n9 i1 i2 i3 i4 i5 i6 i7 i8 i9
PT Sans               | pt_sans_n4              | n7 i4 i7
PT Serif              | pt_serif_n4             | n7 i4 i7
Quattrocento Sans     | quattrocento_sans_n4    | n7 i4 i7
Rakkas                | rakkas_n4               | —
Raleway               | raleway_n4              | n1 n2 n3 n5 n6 n7 n8 n9 i1 i2 i3 i4 i5 i6 i7 i8 i9
Roboto                | roboto_n4               | n1 n3 n5 n7 n9 i1 i3 i4 i5 i7 i9
Roboto Mono           | roboto_mono_n4          | n1 n2 n3 n5 n6 n7 i1 i2 i3 i4 i5 i6 i7
Roboto Slab           | roboto_slab_n4          | n1 n2 n3 n5 n6 n7 n8 n9
Rubik                 | rubik_n4                | n3 n5 n6 n7 n8 n9 i3 i4 i5 i6 i7 i8 i9
Scheherazade New      | scheherazade_new_n4     | n7
Spectral              | spectral_n4             | n2 n3 n5 n6 n7 n8 i2 i3 i4 i5 i6 i7 i8
Tangerine             | tangerine_n4            | n7
Teko                  | teko_n4                 | n3 n5 n6 n7
Ubuntu                | ubuntu_n4               | n3 n5 n7 i3 i4 i5 i7
Unna                  | unna_n4                 | n7 i4 i7
Varela Round          | varela_round_n4         | —
Vollkorn              | vollkorn_n4             | n5 n6 n7 n8 n9 i4 i5 i6 i7 i8 i9
Work Sans             | work_sans_n4            | n1 n2 n3 n5 n6 n7 n8 n9 i1 i2 i3 i4 i5 i6 i7 i8 i9
Yatra One             | yatra_one_n4            | —
```
