import React from "react";
import PostOfficeApp from "./app/PostOfficeApp";
import { PostOfficeProvider } from "./state/PostOfficeContext";

export default function App() {
  return (
    <PostOfficeProvider>
      <PostOfficeApp />
    </PostOfficeProvider>
  );
}
