<script lang="ts">
  import face from './gaffer-companion.png'
  import './gafferCompanionAvatar.css'
  import {gafferAnimations} from './gafferMotion'
  let {size=32,thinking=false,seed=0}:{size?:number;thinking?:boolean;seed?:number}=$props()
</script>
<span class="companion-face" class:thinking class:still={!$gafferAnimations} data-greeting={seed%3} style:width={`${size}px`} style:height={`${size}px`} aria-hidden="true"><img src={face} alt="" width="64" height="64"/></span>
