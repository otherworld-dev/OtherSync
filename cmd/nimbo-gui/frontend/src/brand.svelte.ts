// The product's name as this build is branded (white-label builds have their
// own), for text in any window. It starts as the stock name and takes the
// build's own from App.Brand as soon as the binding answers.
import { App } from "../bindings/github.com/otherworld/nimbo/cmd/nimbo-gui";

export const brand = $state({ name: "OtherSync" });

App.Brand()
  .then((b) => { if (b?.name) brand.name = b.name; })
  .catch(() => { /* keep the stock name */ });
