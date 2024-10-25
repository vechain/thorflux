/* [create-plugin] version: 5.5.3 */
define(["@emotion/css","@grafana/data","@grafana/ui","module","react"],((e,t,r,n,o)=>(()=>{"use strict";var a={89:t=>{t.exports=e},781:e=>{e.exports=t},7:e=>{e.exports=r},308:e=>{e.exports=n},959:e=>{e.exports=o}},s={};function i(e){var t=s[e];if(void 0!==t)return t.exports;var r=s[e]={exports:{}};return a[e](r,r.exports,i),r.exports}i.n=e=>{var t=e&&e.__esModule?()=>e.default:()=>e;return i.d(t,{a:t}),t},i.d=(e,t)=>{for(var r in t)i.o(t,r)&&!i.o(e,r)&&Object.defineProperty(e,r,{enumerable:!0,get:t[r]})},i.o=(e,t)=>Object.prototype.hasOwnProperty.call(e,t),i.r=e=>{"undefined"!=typeof Symbol&&Symbol.toStringTag&&Object.defineProperty(e,Symbol.toStringTag,{value:"Module"}),Object.defineProperty(e,"__esModule",{value:!0})},i.p="public/plugins/vechain-heatmap-panel/";var l={};i.r(l),i.d(l,{plugin:()=>v});var c=i(308),p=i.n(c);i.p=p()&&p().uri?p().uri.slice(0,p().uri.lastIndexOf("/")+1):"public/plugins/vechain-heatmap-panel/";var u=i(781),d=i(959),g=i.n(d),h=i(7),f=i(89);function b(e,t,r){return t in e?Object.defineProperty(e,t,{value:r,enumerable:!0,configurable:!0,writable:!0}):e[t]=r,e}function m(e,t){return t=null!=t?t:{},Object.getOwnPropertyDescriptors?Object.defineProperties(e,Object.getOwnPropertyDescriptors(t)):function(e,t){var r=Object.keys(e);if(Object.getOwnPropertySymbols){var n=Object.getOwnPropertySymbols(e);t&&(n=n.filter((function(t){return Object.getOwnPropertyDescriptor(e,t).enumerable}))),r.push.apply(r,n)}return r}(Object(t)).forEach((function(r){Object.defineProperty(e,r,Object.getOwnPropertyDescriptor(t,r))})),e}const v=new u.PanelPlugin((({data:e,width:t,height:r})=>{const n=(0,h.useTheme2)(),o=(()=>{if(!e.series.length||!e.series[0].fields.length)return[];const t=e.series[0].fields.find((e=>"epoch"===e.name)),r=e.series[0].fields.find((e=>"_value"===e.name));if(!t||!r)return[];const n={};for(let e=0;e<t.values.length;e++){const o=t.values[e],a=r.values[e];n[o]||(n[o]=[]),n[o].push(a)}const o=Object.entries(n).map((([e,t])=>({epoch:parseInt(e),values:t}))).sort(((e,t)=>e.epoch-t.epoch));return o.map(((e,t)=>{const r=0===t,n=1===o.length;if(r&&!n)return{epoch:e.epoch,values:e.values};{const t=[...e.values];for(;t.length<180;)t.push(-1);return{epoch:e.epoch,values:t}}}))})(),a=e=>{if(-1===e)return n.colors.secondary.main;const t=e/100,r=50,o=205,a=50,s=220,i=20,l=60;return`rgb(${Math.round(r+(s-r)*t)}, ${Math.round(o+(i-o)*t)}, ${Math.round(a+(l-a)*t)})`},s={container:f.css`
      padding: ${n.spacing(1)};
      width: 100%;
      height: 100%;
      overflow: auto;
      position: relative;
      isolation: isolate;
    `,row:f.css`
      display: flex;
      align-items: center;
      margin-bottom: ${n.spacing(.5)};
      position: relative;
    `,epochNumber:f.css`
      width: ${60}px;
      margin-right: ${8}px;
      font-size: ${n.typography.size.sm};
    `,blocksContainer:f.css`
      display: flex;
      flex-wrap: wrap;
      gap: ${2}px;
      position: relative;
    `,block:f.css`
      width: ${16}px;
      height: ${16}px;
      border-radius: 2px;
      cursor: pointer;
      transition: opacity 0.2s;
      position: relative;
      &:hover {
        opacity: 0.8;
      }
    `,tooltip:f.css`
      position: absolute;
      background: ${n.colors.background.secondary};
      padding: ${n.spacing(.5)} ${n.spacing(1)};
      border-radius: ${n.shape.radius.default};
      font-size: ${n.typography.size.xs};
      z-index: 9999;
      transform: translate(-50%, 0);
      white-space: nowrap;
      pointer-events: none;
      box-shadow: 0 2px 4px rgba(0, 0, 0, 0.15);
      top: 100%;
      margin-top: 1px;
    `,headerContainer:f.css`
      margin-bottom: ${24}px;
      position: relative;
      height: 20px;
      z-index: 1;
    `,headerContent:f.css`
      position: relative;
      margin-left: ${68}px;
    `,headerMarker:f.css`
      position: absolute;
      text-align: center;
      transform: translateX(-50%);
      color: ${n.colors.text.secondary};
      font-size: ${n.typography.size.sm};
    `},[i,l]=g().useState({visible:!1,text:"",style:{}}),c=e=>18*e+8,p=Array.from({length:Math.ceil(15)},((e,t)=>{const r=12*t;return{blockIndex:r,position:c(r)}}));return g().createElement("div",{className:s.container},g().createElement("div",{className:s.headerContainer},g().createElement("div",{className:s.headerContent},p.map((({blockIndex:e,position:t})=>g().createElement("div",{key:e,className:s.headerMarker,style:{left:t}},e))))),o.map((({epoch:e,values:t})=>g().createElement("div",{key:e,className:s.row},g().createElement("div",{className:s.epochNumber},e),g().createElement("div",{className:s.blocksContainer},t.map(((t,r)=>g().createElement("div",{key:r,className:s.block,style:{backgroundColor:a(t),opacity:-1===t?.3:1},onMouseEnter:n=>((e,t,r,n)=>{var o;const a=e.currentTarget,i=a.getBoundingClientRect(),c=null===(o=a.closest(`.${s.container}`))||void 0===o?void 0:o.getBoundingClientRect();if(!c)return;let p;p=-1===n?"pending":`${n}%`;const u=((e,t)=>180*e+t)(t,r),d=i.left-c.left+i.width/2,g=i.top-c.top+i.height;l({visible:!0,text:`Block ${u}: ${p}`,style:{left:`${d}px`,top:`${g}px`}})})(n,e,r,t),onMouseLeave:()=>l((e=>m(function(e){for(var t=1;t<arguments.length;t++){var r=null!=arguments[t]?arguments[t]:{},n=Object.keys(r);"function"==typeof Object.getOwnPropertySymbols&&(n=n.concat(Object.getOwnPropertySymbols(r).filter((function(e){return Object.getOwnPropertyDescriptor(r,e).enumerable})))),n.forEach((function(t){b(e,t,r[t])}))}return e}({},e),{visible:!1})))}))))))),i.visible&&g().createElement("div",{className:s.tooltip,style:i.style},i.text))})).setPanelOptions((e=>e));return l})()));
//# sourceMappingURL=module.js.map