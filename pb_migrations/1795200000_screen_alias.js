/// <reference path="../pb_data/types.d.ts" />
// Schermen krijgen een alias: een leesbare naam die de web-UI toont in plaats van
// de lange monitornaam/het COM-pad. De client stelt bij het aanmelden van een nieuw
// scherm een redelijke standaard voor; in de web-UI is de alias aan te passen.
migrate(
  (app) => {
    const screens = app.findCollectionByNameOrId("screens");
    screens.fields.add(new TextField({ name: "alias", max: 80, presentable: true }));
    app.save(screens);
  },
  (app) => {
    const screens = app.findCollectionByNameOrId("screens");
    screens.fields.removeByName("alias");
    app.save(screens);
  }
);
