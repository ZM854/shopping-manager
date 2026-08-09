import Button from "../../components/UI/button/Button/Button";
import { useAuth } from "../../hooks/useAuth";
import cls from "./ProfilePage.module.css";

const avatarColors = [
  "#629838",
  "#4F7CAC",
  "#9B6B9E",
  "#C17C4A",
  "#4D8B8B",
  "#B05A5A",
];

const ProfilePage = () => {
  const { user, logout } = useAuth();

  if (!user) {
    return null;
  }

  let hash = 0;

  for (let i = 0; i < user.name.length; i++) {
    hash += user.name.charCodeAt(i);
  }

  const avatarColor = avatarColors[hash % avatarColors.length];

  const firstLetter = user.name.charAt(0).toUpperCase();

  return (
    <div className={cls.profile}>
      <div className={cls.info}>
        <div className={cls.avatar} style={{ backgroundColor: avatarColor }}>
          {firstLetter}
        </div>
        <h1 className={cls.name}>{user.name}</h1>
      </div>
      <div className={cls.actions}>
        <Button onClick={logout}>Выйти из аккаунта</Button>
      </div>
    </div>
  );
};

export default ProfilePage;
