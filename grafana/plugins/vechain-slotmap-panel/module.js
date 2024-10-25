/* [create-plugin] version: 5.3.11 */
define(["@emotion/css","@grafana/data","@grafana/ui","module","react"],((e,t,r,o,n)=>(()=>{"use strict";var s={89:t=>{t.exports=e},781:e=>{e.exports=t},7:e=>{e.exports=r},308:e=>{e.exports=o},959:e=>{e.exports=n}},a={};function i(e){var t=a[e];if(void 0!==t)return t.exports;var r=a[e]={exports:{}};return s[e](r,r.exports,i),r.exports}i.n=e=>{var t=e&&e.__esModule?()=>e.default:()=>e;return i.d(t,{a:t}),t},i.d=(e,t)=>{for(var r in t)i.o(t,r)&&!i.o(e,r)&&Object.defineProperty(e,r,{enumerable:!0,get:t[r]})},i.o=(e,t)=>Object.prototype.hasOwnProperty.call(e,t),i.r=e=>{"undefined"!=typeof Symbol&&Symbol.toStringTag&&Object.defineProperty(e,Symbol.toStringTag,{value:"Module"}),Object.defineProperty(e,"__esModule",{value:!0})},i.p="public/plugins/vechain-slotmap-panel/";var l={};i.r(l),i.d(l,{plugin:()=>b});var c=i(308),p=i.n(c);i.p=p()&&p().uri?p().uri.slice(0,p().uri.lastIndexOf("/")+1):"public/plugins/vechain-slotmap-panel/";var u=i(781),d=i(959),m=i.n(d),h=i(7),f=i(89);function g(e,t,r){return t in e?Object.defineProperty(e,t,{value:r,enumerable:!0,configurable:!0,writable:!0}):e[t]=r,e}function v(e,t){return t=null!=t?t:{},Object.getOwnPropertyDescriptors?Object.defineProperties(e,Object.getOwnPropertyDescriptors(t)):function(e,t){var r=Object.keys(e);if(Object.getOwnPropertySymbols){var o=Object.getOwnPropertySymbols(e);t&&(o=o.filter((function(t){return Object.getOwnPropertyDescriptor(e,t).enumerable}))),r.push.apply(r,o)}return r}(Object(t)).forEach((function(r){Object.defineProperty(e,r,Object.getOwnPropertyDescriptor(t,r))})),e}const b=new u.PanelPlugin((({data:e,width:t,height:r})=>{const o=(0,h.useTheme2)(),{epochs:n,maxSlots:s}=(()=>{if(!e.series.length||!e.series[0].fields.length)return{epochs:[],maxSlots:180};const t=e.series[0].fields.find((e=>"epoch"===e.name)),r=e.series[0].fields.find((e=>"_value"===e.name));if(!t||!r)return{epochs:[],maxSlots:180};const o={};let n=180;for(let e=0;e<t.values.length;e++){const s=t.values[e],a=r.values[e];o[s]||(o[s]=[]),o[s].push(a),n=Math.max(n,o[s].length)}const s=Object.entries(o).map((([e,t])=>({epoch:parseInt(e),values:t}))).sort(((e,t)=>e.epoch-t.epoch)),a=s.map(((e,t)=>{const r=0===t,o=1===s.length;if(r&&!o)return{epoch:e.epoch,values:e.values};{const t=[...e.values];for(;t.length<n;)t.push(-1);return{epoch:e.epoch,values:t}}}));return{epochs:a,maxSlots:n}})(),a=e=>{switch(e){case 1:return o.colors.success.main;case 0:return o.colors.error.main;default:return o.colors.secondary.main}},i={container:f.css`
      padding: ${o.spacing(1)};
      width: 100%;
      height: 100%;
      overflow: auto;
      position: relative;
      isolation: isolate;
    `,row:f.css`
      display: flex;
      align-items: center;
      margin-bottom: ${o.spacing(.5)};
      position: relative;
    `,epochNumber:f.css`
      width: ${60}px;
      margin-right: ${8}px;
      font-size: ${o.typography.size.sm};
    `,slotsContainer:f.css`
      display: flex;
      flex-wrap: wrap;
      gap: ${2}px;
      position: relative;
    `,slot:f.css`
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
      background: ${o.colors.background.secondary};
      padding: ${o.spacing(.5)} ${o.spacing(1)};
      border-radius: ${o.shape.radius.default};
      font-size: ${o.typography.size.xs};
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
      color: ${o.colors.text.secondary};
      font-size: ${o.typography.size.sm};
    `},[l,c]=m().useState({visible:!1,text:"",style:{}}),p=(e,t,r,o)=>{var n;const a=e.currentTarget,l=a.getBoundingClientRect(),p=null===(n=a.closest(`.${i.container}`))||void 0===n?void 0:n.getBoundingClientRect();if(!p)return;const u=1===o?"filled":0===o?"missed":"pending",d=((e,t)=>e*s+t)(t,r),m=l.left-p.left+l.width/2,h=l.top-p.top+l.height;c({visible:!0,text:`Slot ${d}: ${u}`,style:{left:`${m}px`,top:`${h}px`}})},u=e=>18*e+8,d=Array.from({length:Math.ceil(s/12)},((e,t)=>{const r=12*t;return{slotIndex:r,position:u(r)}}));return m().createElement("div",{className:i.container},m().createElement("div",{className:i.headerContainer},m().createElement("div",{className:i.headerContent},d.map((({slotIndex:e,position:t})=>m().createElement("div",{key:e,className:i.headerMarker,style:{left:t}},e))))),n.map((({epoch:e,values:t})=>m().createElement("div",{key:e,className:i.row},m().createElement("div",{className:i.epochNumber},e),m().createElement("div",{className:i.slotsContainer},t.map(((t,r)=>m().createElement("div",{key:r,className:i.slot,style:{backgroundColor:a(t),opacity:-1===t?.3:1},onMouseEnter:o=>p(o,e,r,t),onMouseLeave:()=>c((e=>v(function(e){for(var t=1;t<arguments.length;t++){var r=null!=arguments[t]?arguments[t]:{},o=Object.keys(r);"function"==typeof Object.getOwnPropertySymbols&&(o=o.concat(Object.getOwnPropertySymbols(r).filter((function(e){return Object.getOwnPropertyDescriptor(r,e).enumerable})))),o.forEach((function(t){g(e,t,r[t])}))}return e}({},e),{visible:!1})))}))))))),l.visible&&m().createElement("div",{className:i.tooltip,style:l.style},l.text))})).setPanelOptions((e=>e));return l})()));
//# sourceMappingURL=module.js.map